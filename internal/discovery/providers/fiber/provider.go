// Package fiber implements a conservative Fiber (github.com/gofiber/fiber)
// route-scan discovery provider (plan.md v8: "Framework providers: ...
// Fiber ..."). Same conservative approach as internal/discovery/providers/
// gin: never executes user Go code, never invents endpoints not present in
// source (docs/11-risks-and-gaps.md R2). Duplicated rather than shared
// with the gin provider per docs/03-plugins.md section 4's isolation rule.
package fiber

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

// appVarPattern finds "app := fiber.New()".
var appVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*fiber\.New\s*\(`)

// groupVarPattern finds "api := app.Group("/api")" — composes across
// nested Group() calls the same way gin's does.
var groupVarPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\s*:=\s*([a-zA-Z_][a-zA-Z0-9_]*)\.Group\s*\(\s*"([^"]*)"`)

// routeCallPattern matches "<receiver>.Get("/path", ...)" style calls.
// Fiber's HTTP method helpers are capitalized-first (Get/Post/Put/Patch/
// Delete/Options/Head/All).
var routeCallPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_][a-zA-Z0-9_]*)\.(Get|Post|Put|Patch|Delete|Options|Head|All)\s*\(\s*"([^"]*)"`)

// dynamicCallPattern mirrors gin's: same call shape with a non-literal
// first argument.
var dynamicCallPattern = regexp.MustCompile(
	`(?m)\b[a-zA-Z_][a-zA-Z0-9_]*\.(Get|Post|Put|Patch|Delete|Options|Head|All)\s*\(\s*([a-zA-Z_][a-zA-Z0-9_.]*)\s*[,)]`)

// Provider implements discovery.Provider for Fiber source scanning.
type Provider struct{}

// New builds a Fiber provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "fiber" }

// Detect checks go.mod for a gofiber/fiber require line.
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "go.mod")
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(data), "github.com/gofiber/fiber"), nil
}

// Discover walks *.go files, extracts route calls, resolves same-file
// app.Group()/nested group prefixes, and skips dynamic calls.
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
	appVars := map[string]bool{}
	for _, m := range appVarPattern.FindAllStringSubmatch(source, -1) {
		appVars[m[1]] = true
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
		if appVars[name] {
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
		if method == "ALL" {
			method = "GET" // conservative, same rationale as gin's Any()/express's .all()
		}
		prefix, ok := resolvePrefix(receiver, map[string]bool{})
		if !ok {
			continue
		}
		fullPath := discovery.JoinURLPath(prefix, routePath)
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(method),
			Path:          fullPath,
			Sources:       []string{"fiber"},
			PrimarySource: "fiber",
		})
	}
	return endpoints
}
