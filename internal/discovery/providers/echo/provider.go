// Package echo implements a conservative Echo (github.com/labstack/echo)
// route-scan discovery provider (plan.md v8: "Framework providers: ...
// Echo ..."). Same conservative approach as the gin/fiber providers:
// never executes user Go code, never invents endpoints not present in
// source (docs/11-risks-and-gaps.md R2). Duplicated rather than shared
// per docs/03-plugins.md section 4's isolation rule.
package echo

import (
	"context"
	"io/fs"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
)

const maxWalkDepth = 6

var sourceExtensions = []string{".go"}

// engineVarPattern finds "e := echo.New()".
var engineVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*echo\.New\s*\(\s*\)`)

// groupVarPattern finds "api := e.Group("/api")".
var groupVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*([a-zA-Z_][a-zA-Z0-9_]*)\.Group\s*\(\s*"([^"]*)"`)

// routeCallPattern matches "<receiver>.GET("/path", ...)" style calls.
// Echo's HTTP method helpers are capitalized (GET/POST/PUT/PATCH/DELETE/
// OPTIONS/HEAD/Any).
var routeCallPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\s*\(\s*"([^"]*)"`)

// dynamicCallPattern mirrors gin/fiber's.
var dynamicCallPattern = regexp.MustCompile(
	`(?m)\b[a-zA-Z_][a-zA-Z0-9_]*\.(GET|POST|PUT|PATCH|DELETE|OPTIONS|HEAD|Any)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*[,)]`)

// Provider implements discovery.Provider for Echo source scanning.
type Provider struct{}

// New builds an Echo provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "echo" }

// Detect checks go.mod for a labstack/echo require line.
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "go.mod")
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(data), "github.com/labstack/echo"), nil
}

// Discover walks *.go files, extracts route calls, resolves same-file
// e.Group()/nested group prefixes, and skips dynamic calls.
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
			continue
		}
		endpoints = append(endpoints, extractRoutes(string(data))...)
	}
	return endpoints, nil
}

// SkippedDynamicCount re-scans the same source for dynamic route calls.
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

func extractRoutes(source string) []domain.Endpoint {
	engineVars := map[string]bool{}
	for _, m := range engineVarPattern.FindAllStringSubmatch(source, -1) {
		engineVars[m[1]] = true
	}

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
			return "", false
		}
		if seen[name] {
			return "", false
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
			method = "GET" // conservative, same rationale as gin/fiber
		}
		prefix, ok := resolvePrefix(receiver, map[string]bool{})
		if !ok {
			continue
		}
		fullPath := discovery.JoinURLPath(prefix, routePath)
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(method),
			Path:          fullPath,
			Sources:       []string{"echo"},
			PrimarySource: "echo",
		})
	}
	return endpoints
}
