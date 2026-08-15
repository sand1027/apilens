package nestjs

import (
	"context"
	"os"
	"testing"

	"github.com/sandeepv/apilens/internal/discovery"
)

func TestDetect_TrueWhenPackageJSONHasNestjsCore(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/nestjs/simple")
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Error("expected Detect to find @nestjs/core dependency in package.json")
	}
}

func TestDetect_FalseWithoutPackageJSON(t *testing.T) {
	p := New()
	root := os.DirFS(t.TempDir())
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ok {
		t.Error("expected Detect to be false without package.json")
	}
}

func TestDiscover_SimpleFixture_FindsControllerRouteOnly(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/nestjs/simple")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"health.controller.ts"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	if len(endpoints) != 1 {
		t.Fatalf("expected exactly 1 endpoint (the @Controller route only), got %d: %+v", len(endpoints), endpoints)
	}
	if string(endpoints[0].Method) != "GET" || endpoints[0].Path != "/health" {
		t.Errorf("unexpected endpoint: %+v", endpoints[0])
	}
}

func TestDiscover_NeverCountsMethodsOnNonControllerClasses(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/nestjs/simple")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"health.controller.ts"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, ep := range endpoints {
		if ep.Path == "/should-not-count" {
			t.Errorf("a @Get() on a class without @Controller() must not be treated as a route: %+v", ep)
		}
	}
}

func TestDiscover_PrefixedFixture_AppliesControllerPrefixAndParamPaths(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/nestjs/prefixed")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"users.controller.ts"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := map[string]bool{
		"GET /api/users":     true,
		"GET /api/users/:id": true,
		"POST /api/users":    true,
	}
	found := map[string]bool{}
	for _, ep := range endpoints {
		found[string(ep.Method)+" "+ep.Path] = true
	}
	for k := range want {
		if !found[k] {
			t.Errorf("expected endpoint %q, got %+v", k, endpoints)
		}
	}
	if len(endpoints) != len(want) {
		t.Errorf("expected exactly %d endpoints, got %d: %+v", len(want), len(endpoints), endpoints)
	}
}

func TestExtractRoutes_BareDecoratorPathIsJustPrefix(t *testing.T) {
	source := `
@Controller('items')
export class ItemsController {
  @Get()
  list() { return []; }
}
`
	endpoints := extractRoutes(source)
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint, got %d: %+v", len(endpoints), endpoints)
	}
	if endpoints[0].Path != "/items" {
		t.Errorf("Path = %q, want /items", endpoints[0].Path)
	}
}
