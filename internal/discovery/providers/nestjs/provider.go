// Package nestjs implements a conservative NestJS (@nestjs/core)
// route-scan discovery provider (plan.md v8: "Framework providers: ...
// NestJS ..."). NestJS routes are declared via TypeScript decorators
// (@Controller('/prefix') on a class, @Get('/path')/@Post('/path')/etc on
// its methods) rather than direct function calls, so this provider scans
// for decorator syntax instead of the call-expression regex the other
// providers use — but keeps the same discipline: never invents an
// endpoint for a class without an explicit @Controller() decorator, and
// never guesses at a non-literal decorator argument
// (docs/11-risks-and-gaps.md R2).
package nestjs

import (
	"context"
	"io/fs"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/domain"
)

const maxWalkDepth = 6

var sourceExtensions = []string{".ts"}

// controllerPattern matches "@Controller('prefix')" or "@Controller()"
// immediately followed (across whitespace/newlines) by a class
// declaration — capturing the optional prefix and the class body start.
// The prefix group is empty for a bare @Controller().
var controllerPattern = regexp.MustCompile(
	`(?ms)@Controller\s*\(\s*(?:['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `])?\s*\)\s*(?:export\s+)?class\s+[a-zA-Z_$][a-zA-Z0-9_]*\s*\{`)

// methodDecoratorPattern matches "@Get('path')", "@Post()", etc. inside a
// controller class body. An empty argument list means the route path is
// just the controller's own prefix.
var methodDecoratorPattern = regexp.MustCompile(
	`(?m)@(Get|Post|Put|Patch|Delete|Options|Head|All)\s*\(\s*(?:['"` + "`" + `]([^'"` + "`" + `]*)['"` + "`" + `])?\s*\)`)

// Provider implements discovery.Provider for NestJS source scanning.
type Provider struct{}

// New builds a NestJS provider.
func New() *Provider { return &Provider{} }

func (p *Provider) Name() string { return "nestjs" }

// Detect checks package.json for a "@nestjs/core" dependency.
func (p *Provider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	data, err := fs.ReadFile(root, "package.json")
	if err != nil {
		return false, nil
	}
	return strings.Contains(string(data), `"@nestjs/core"`), nil
}

// Discover walks *.ts files, finds each @Controller class body, and
// extracts its @Get/@Post/etc method decorators, applying the
// controller's prefix.
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

// extractRoutes finds every @Controller class in source and scans only
// the text between that class's opening "{" and its matching closing "}"
// for method decorators — this is what prevents a @Get() on some
// unrelated, non-controller class (or a helper function) from being
// treated as a route (the NestJS equivalent of express's receiver
// allow-list / R2's "never invent" rule).
func extractRoutes(source string) []domain.Endpoint {
	var endpoints []domain.Endpoint

	controllerMatches := controllerPattern.FindAllStringSubmatchIndex(source, -1)
	for _, idx := range controllerMatches {
		prefix := ""
		if idx[2] != -1 {
			prefix = source[idx[2]:idx[3]]
		}
		bodyStart := idx[1] // position right after the class's opening "{"
		bodyEnd := matchingBrace(source, bodyStart-1)
		if bodyEnd == -1 {
			continue // unbalanced braces — malformed/partial source, skip rather than guess
		}
		body := source[bodyStart:bodyEnd]

		for _, m := range methodDecoratorPattern.FindAllStringSubmatch(body, -1) {
			method, routePath := strings.ToUpper(m[1]), m[2]
			if method == "ALL" {
				method = "GET" // conservative, same rationale as every other provider's "match-all" call
			}
			fullPath := discovery.JoinURLPath(prefix, routePath)
			endpoints = append(endpoints, domain.Endpoint{
				Method:        domain.NormalizeMethod(method),
				Path:          fullPath,
				Sources:       []string{"nestjs"},
				PrimarySource: "nestjs",
			})
		}
	}
	return endpoints
}

// matchingBrace scans forward from the index of an opening "{" (openIdx)
// and returns the index of its matching closing "}", or -1 if the braces
// never balance (a malformed/truncated file). This is a simple depth
// counter, not a full TS parser — adequate for finding a class body's
// extent without pulling in a TypeScript AST dependency.
func matchingBrace(s string, openIdx int) int {
	if openIdx < 0 || openIdx >= len(s) || s[openIdx] != '{' {
		return -1
	}
	depth := 0
	for i := openIdx; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}
