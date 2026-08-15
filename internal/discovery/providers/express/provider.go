// Package express implements the conservative Express.js route-scan
// discovery provider (docs/07-discovery.md section 4). It never executes
// user JavaScript and never invents endpoints not present in source
// (docs/11-risks-and-gaps.md R2).
package express

import (
	"context"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
)

// walkIgnoreDirs mirrors the openapi provider's ignore list
// (docs/07-discovery.md section 3/4, docs/11-risks-and-gaps.md R12).
var walkIgnoreDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true,
	"dist": true, "build": true, ".apilens": true,
}

// maxWalkDepth caps the source scan, same rationale as the OpenAPI
// provider's walk.
const maxWalkDepth = 6

// routeCallPattern matches "<identifier>.get('/path', ...)" style calls —
// the conservative regex scan from docs/07-discovery.md section 4 step 1.
// It intentionally only matches a string-literal first argument; anything
// else (a variable, a template expression) is left for dynamicCallPattern
// to count as skipped. The identifier is captured so the caller can decide
// whether it's "app", "router", or a named router variable that might have
// a resolved mount prefix.
var routeCallPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_$][a-zA-Z0-9_]*)\.(get|post|put|patch|delete|all)\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `]`)

// dynamicCallPattern matches the same call shape but with a non-string
// first argument (a variable or expression), so we can report
// skipped_dynamic without inventing a path for it
// (docs/07-discovery.md section 4 step 3).
var dynamicCallPattern = regexp.MustCompile(
	`(?m)\b(?:app|router|[a-zA-Z_][a-zA-Z0-9_]*Router)\[?\.?(get|post|put|patch|delete|all)\]?\s*\(\s*([a-zA-Z_$][a-zA-Z0-9_.]*)\s*[,)]`)

// mountPattern matches "app.use('/api', someRouter)" style mounts so a
// same-file router can be prefixed (docs/07-discovery.md section 4 step 2).
// Only the simple "identifier" target form is resolved; anything else is
// left unmounted (still discovered, just without the prefix) rather than
// guessed at.
var mountPattern = regexp.MustCompile(
	`(?m)\b(?:app|router)\.use\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `]\s*,\s*([a-zA-Z_$][a-zA-Z0-9_]*)\s*\)`)

// routerVarPattern finds "const userRouter = express.Router()" so we know
// which identifiers are routers worth scanning for mounts.
var routerVarPattern = regexp.MustCompile(
	`(?m)(?:const|let|var)\s+([a-zA-Z_$][a-zA-Z0-9_]*)\s*=\s*express\.Router\s*\(`)

// Provider implements discovery.Provider for Express.js source scanning.
type Provider struct{}

// New builds an Express provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "express" }

// Detect checks package.json for an "express" dependency — cheap, no
// source parsing (docs/03-plugins.md section 4).
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "package.json")
	if err != nil {
		return false, nil // no package.json is not an error, just "not Express"
	}
	return hasExpressDependency(data), nil
}

// Discover walks *.js/*.ts/*.mjs/*.cjs files (depth-limited, ignoring
// node_modules etc.), extracts route calls per file, resolves same-file
// router.use mounts, and reports dynamic calls as skipped rather than
// guessing (docs/07-discovery.md section 4).
func (p *Provider) Discover(ctx context.Context, root fs.FS, opts discovery.Options) ([]domain.Endpoint, error) {
	var files []string
	if len(opts.Paths) > 0 {
		files = opts.Paths
	} else {
		var err error
		files, err = walkForSources(root)
		if err != nil {
			return nil, err
		}
	}

	var endpoints []domain.Endpoint
	for _, f := range files {
		data, err := fs.ReadFile(root, f)
		if err != nil {
			continue // unreadable file: skip, don't abort (R2: never invent, but also don't crash on one bad file)
		}
		endpoints = append(endpoints, extractRoutes(string(data))...)
	}
	return endpoints, nil
}

// SkippedDynamicCount re-scans the same source for dynamic route calls
// (e.g. app[method](variable)) so the CLI can print
// "skipped_dynamic: N" per docs/07-discovery.md section 4. Kept as a
// separate pass so Discover's return type stays []domain.Endpoint per the
// Provider contract; the orchestrator/CLI layer calls this directly on the
// same file set when it wants the count.
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

func hasExpressDependency(packageJSON []byte) bool {
	// Avoid a full JSON unmarshal dependency chain for a one-line check;
	// a simple substring search is deliberately conservative here since
	// we only use this as a Detect() hint, not as ground truth.
	s := string(packageJSON)
	return strings.Contains(s, `"express"`)
}

func walkForSources(root fs.FS) ([]string, error) {
	var results []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if p == "." {
			return nil
		}
		depth := strings.Count(p, "/") + 1
		if d.IsDir() {
			if walkIgnoreDirs[d.Name()] || depth > maxWalkDepth {
				return fs.SkipDir
			}
			return nil
		}
		if depth > maxWalkDepth {
			return nil
		}
		ext := path.Ext(p)
		if ext == ".js" || ext == ".ts" || ext == ".mjs" || ext == ".cjs" {
			results = append(results, p)
		}
		return nil
	})
	sort.Strings(results)
	return results, err
}

// extractRoutes implements docs/07-discovery.md section 4's per-file
// extraction: find router.use mounts for locally-declared routers, then
// find route calls and apply a mount prefix when the call target is a
// known mounted router variable declared in the same file.
func extractRoutes(source string) []domain.Endpoint {
	routerVars := map[string]bool{}
	for _, m := range routerVarPattern.FindAllStringSubmatch(source, -1) {
		routerVars[m[1]] = true
	}

	mountPrefix := map[string]string{} // router var name -> mount prefix
	for _, m := range mountPattern.FindAllStringSubmatch(source, -1) {
		prefix, varName := m[1], m[2]
		if routerVars[varName] {
			mountPrefix[varName] = prefix
		}
	}

	var endpoints []domain.Endpoint
	for _, m := range routeCallPattern.FindAllStringSubmatch(source, -1) {
		receiver, method, routePath := m[1], strings.ToUpper(m[2]), m[3]
		if method == "ALL" {
			// "all" registers every method; represent it as GET for
			// discovery purposes rather than inventing 5 endpoints from
			// one ambiguous call — conservative per R2.
			method = "GET"
		}
		// Only "app", "router", and locally-declared express.Router()
		// variables are treated as real route registrations — an
		// arbitrary object named e.g. "myLogger.get(...)" would otherwise
		// produce a false positive (docs/11-risks-and-gaps.md R2).
		if receiver != "app" && receiver != "router" && !routerVars[receiver] {
			continue
		}
		routePath = joinPath(mountPrefix[receiver], routePath)
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(method),
			Path:          routePath,
			Sources:       []string{"express"},
			PrimarySource: "express",
		})
	}
	return endpoints
}

func joinPath(prefix, suffix string) string {
	var p string
	if prefix == "" {
		p = suffix
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	} else {
		p = strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(suffix, "/")
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	}
	// A mounted router's own "/" root path should collapse onto the
	// mount prefix itself (e.g. prefix "/api/users" + suffix "/" ==
	// "/api/users", not "/api/users/").
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
