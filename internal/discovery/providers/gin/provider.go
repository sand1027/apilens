// Package gin implements a conservative Gin (github.com/gin-gonic/gin)
// route-scan discovery provider (plan.md v8: "Framework providers: ...
// Gin ..."). It never executes user Go code and never invents endpoints
// not present in source (docs/11-risks-and-gaps.md R2), mirroring the
// Express provider's approach for a Go source tree instead of JS
// (docs/03-plugins.md section 4: "providers/express must not import
// providers/openapi" — the same isolation applies here, so this package
// duplicates the small amount of Gin-specific regex logic rather than
// depending on another provider package).
package gin

import (
	"context"
	"io/fs"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
)

// maxWalkDepth caps the source scan (same rationale as every other
// walk-based provider — docs/11-risks-and-gaps.md R12).
const maxWalkDepth = 6

var sourceExtensions = []string{".go"}

// engineVarPattern finds "r := gin.Default()" / "router := gin.New()" so
// we know which identifiers are the top-level engine, worth scanning for
// route calls and groups.
var engineVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*gin\.(?:Default|New)\s*\(\s*\)`)

// groupVarPattern finds "api := r.Group("/api")" — the receiver may be the
// engine itself or another already-declared group variable, so prefixes
// compose across multiple Group() calls.
var groupVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*([a-zA-Z_][a-zA-Z0-9_]*)\.Group\s*\(\s*"([^"]*)"`)

// routeCallPattern matches "<receiver>.GET("/path", ...)" style calls with
// a string-literal path. Gin's HTTP method helpers are capitalized
// (GET/POST/PUT/PATCH/DELETE/OPTIONS/HEAD/Any).
var routeCallPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\s*\(\s*"([^"]*)"`)

// dynamicCallPattern matches the same call shape but with a non-literal
// first argument (a variable), so it can be counted as skipped rather than
// guessed at (docs/07-discovery.md section 4 step 3, applied to Go).
var dynamicCallPattern = regexp.MustCompile(
	`(?m)\b[a-zA-Z_][a-zA-Z0-9_]*\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*[,)]`)

// Provider implements discovery.Provider for Gin source scanning.
type Provider struct{}

// New builds a Gin provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "gin" }

// Detect checks go.mod for a gin-gonic/gin require line — cheap, no
// source parsing (docs/03-plugins.md section 4).
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "go.mod")
	if err != nil {
		return false, nil // no go.mod is not an error, just "not a Go module"
	}
	return strings.Contains(string(data), "github.com/gin-gonic/gin"), nil
}

// Discover walks *.go files (depth-limited, ignoring vendor/.git/etc.),
// extracts route calls per file, resolves same-file r.Group()/nested
// group prefixes, and skips dynamic (non-literal-path) calls rather than
// guessing.
func (p *Provider) Discover(ctx context.Context, root fs.FS, opts discovery.Options) ([]domain.Endpoint, error) {
	var files []string
	if len(opts.Paths) > 0 {
		files = opts.Paths
	} else {
		var err error
		files, err = discovery.WalkSourceFiles(root, sourceExtensions, discovery.DefaultIgnoreDirs, maxWalkDepth)
		if err != nil {
			return nil, err
		}
	}

	var endpoints []domain.Endpoint
	for _, f := range files {
		data, err := fs.ReadFile(root, f)
		if err != nil {
			continue // unreadable file: skip, don't abort
		}
		endpoints = append(endpoints, extractRoutes(string(data))...)
	}
	return endpoints, nil
}

// SkippedDynamicCount re-scans the same source for dynamic route calls,
// mirroring the express provider's exported helper of the same name and
// purpose.
func SkippedDynamicCount(root fs.FS, files []string) (int, error) {
	total := 0
	for _, f := range files {
		data, err := fs.ReadFile(root, f)
		if err != nil {
			continue
		}
		total += len(dynamicCallPattern.FindAllString(string(data), -1))
	}
	return total, nil
}

// extractRoutes finds engine vars, resolves group prefixes (composing
// across nested Group() calls), then finds route calls and applies the
// resolved prefix for whichever receiver made the call.
func extractRoutes(source string) []domain.Endpoint {
	engineVars := map[string]bool{}
	for _, m := range engineVarPattern.FindAllStringSubmatch(source, -1) {
		engineVars[m[1]] = true
	}

	// groupParent maps a group var to its (receiver, own prefix segment);
	// resolvePrefix below walks this chain to compose the full prefix.
	type groupInfo struct {
		receiver string
		prefix   string
	}
	groups := map[string]groupInfo{}
	for _, m := range groupVarPattern.FindAllStringSubmatch(source, -1) {
		varName, receiver, prefix := m[1], m[2], m[3]
		groups[varName] = groupInfo{receiver: receiver, prefix: prefix}
	}

	var resolvePrefix func(name string, seen map[string]bool) (string, bool)
	resolvePrefix = func(name string, seen map[string]bool) (string, bool) {
		if engineVars[name] {
			return "", true
		}
		g, ok := groups[name]
		if !ok {
			return "", false // unknown receiver — never guessed at
		}
		if seen[name] {
			return "", false // cycle guard
		}
		seen[name] = true
		parentPrefix, ok := resolvePrefix(g.receiver, seen)
		if !ok {
			return "", false
		}
		return discovery.JoinURLPath(parentPrefix, g.prefix), true
	}

	var endpoints []domain.Endpoint
	for _, m := range routeCallPattern.FindAllStringSubmatch(source, -1) {
		receiver, method, routePath := m[1], strings.ToUpper(m[2]), m[3]
		if method == "ANY" {
			// Any() registers every method — represent as GET rather than
			// inventing several endpoints from one ambiguous call (R2,
			// same conservative choice the express provider makes for
			// .all()).
			method = "GET"
		}
		prefix, ok := resolvePrefix(receiver, map[string]bool{})
		if !ok {
			// Receiver is neither a known engine var nor a known group —
			// e.g. some unrelated object with a coincidentally-named GET
			// method. Skip rather than risk a false positive (R2).
			continue
		}
		fullPath := discovery.JoinURLPath(prefix, routePath)
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(method),
			Path:          fullPath,
			Sources:       []string{"gin"},
			PrimarySource: "gin",
		})
	}
	return endpoints
}
