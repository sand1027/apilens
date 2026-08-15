// Package fastify implements a conservative Fastify route-scan discovery
// provider (plan.md v8: "Framework providers: Fastify ..."). Same
// conservative approach as internal/discovery/providers/express: never
// executes user JavaScript, never invents endpoints not present in
// source (docs/11-risks-and-gaps.md R2). Duplicated rather than shared
// with the express provider per docs/03-plugins.md section 4's isolation
// rule.
package fastify

import (
	"context"
	"io/fs"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
)

const maxWalkDepth = 6

var sourceExtensions = []string{".js", ".ts", ".mjs", ".cjs"}

// instanceVarPattern finds "const fastify = require('fastify')()" or
// "const app = Fastify()" — the common ways a Fastify instance variable is
// created, so route calls on it (and only it, plus the literal "fastify"
// identifier convention) are trusted.
var instanceVarPattern = regexp.MustCompile(
	`(?m)(?:const|let|var)\s+([a-zA-Z_$][a-zA-Z0-9_]*)\s*=\s*(?:require\(['"` + "`" + `]fastify['"` + "`" + `]\)|Fastify)\s*\(`)

// routeCallPattern matches "<receiver>.get('/path', ...)" and
// "<receiver>.route({ method: 'GET', url: '/path', ... })" style calls.
// Only the simpler .get/.post/etc form is handled by this regex; the
// object-form .route() call is handled separately by routeObjectPattern.
var routeCallPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_$][a-zA-Z0-9_]*)\.(get|post|put|patch|delete|head|options)\s*\(\s*['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `]`)

// routeObjectPattern matches "<receiver>.route({ method: 'GET', url:
// '/path' ...})" — order of method/url keys is not assumed; both are
// looked up independently within the matched object literal text.
var routeObjectPattern = regexp.MustCompile(
	`(?m)\b([a-zA-Z_$][a-zA-Z0-9_]*)\.route\s*\(\s*\{([^}]*)\}`)
var methodKeyPattern = regexp.MustCompile(`method\s*:\s*['"` + "`" + `]([a-zA-Z]+)['"` + "`" + `]`)
var urlKeyPattern = regexp.MustCompile(`url\s*:\s*['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `]`)

// dynamicCallPattern matches the .get/.post/etc call shape with a
// non-literal first argument.
var dynamicCallPattern = regexp.MustCompile(
	`(?m)\b[a-zA-Z_$][a-zA-Z0-9_]*\.(get|post|put|patch|delete|head|options)\s*\(\s*([a-zA-Z_$][a-zA-Z0-9_.]*)\s*[,)]`)

// Provider implements discovery.Provider for Fastify source scanning.
type Provider struct{}

// New builds a Fastify provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "fastify" }

// Detect checks package.json for a "fastify" dependency.
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "package.json")
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(data), `"fastify"`), nil
}

// Discover walks *.js/*.ts/*.mjs/*.cjs files, extracts both call forms
// (.get/.post/etc and .route({...})), and skips dynamic calls.
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

// extractRoutes finds Fastify instance vars, then extracts both the
// .get/.post/etc call form and the .route({...}) object form — only for
// receivers known to be a Fastify instance (or the conventional
// "fastify"/"app"/"server" identifiers), same false-positive guard as the
// express provider's receiver allow-list.
func extractRoutes(source string) []domain.Endpoint {
	instanceVars := map[string]bool{"fastify": true, "app": true, "server": true}
	for _, m := range instanceVarPattern.FindAllStringSubmatch(source, -1) {
		instanceVars[m[1]] = true
	}

	var endpoints []domain.Endpoint
	for _, m := range routeCallPattern.FindAllStringSubmatch(source, -1) {
		receiver, method, routePath := m[1], strings.ToUpper(m[2]), m[3]
		if !instanceVars[receiver] {
			continue
		}
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(method),
			Path:          routePath,
			Sources:       []string{"fastify"},
			PrimarySource: "fastify",
		})
	}

	for _, m := range routeObjectPattern.FindAllStringSubmatch(source, -1) {
		receiver, body := m[1], m[2]
		if !instanceVars[receiver] {
			continue
		}
		methodMatch := methodKeyPattern.FindStringSubmatch(body)
		urlMatch := urlKeyPattern.FindStringSubmatch(body)
		if methodMatch == nil || urlMatch == nil {
			continue // incomplete object literal (e.g. spread from elsewhere) — never guessed at
		}
		endpoints = append(endpoints, domain.Endpoint{
			Method:        domain.NormalizeMethod(methodMatch[1]),
			Path:          urlMatch[1],
			Sources:       []string{"fastify"},
			PrimarySource: "fastify",
		})
	}
	return endpoints
}
