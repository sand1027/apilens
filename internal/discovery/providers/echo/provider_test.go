package echo

import (
	"context"
	"os"
	"testing"

	"github.com/sandeepv/apilens/internal/discovery"
)

func TestDetect_TrueWhenGoModHasEcho(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/echo/simple")
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Error("expected Detect to find labstack/echo in go.mod")
	}
}

func TestDetect_FalseWithoutGoMod(t *testing.T) {
	p := New()
	root := os.DirFS(t.TempDir())
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ok {
		t.Error("expected Detect to be false without go.mod")
	}
}

func TestDiscover_SimpleFixture_FindsStaticRoutesOnly(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/echo/simple")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"main.go"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := map[string]bool{
		"GET /health":     true,
		"GET /api/users":  true,
		"POST /api/users": true,
	}
	if len(endpoints) != len(want) {
		t.Fatalf("expected %d endpoints, got %d: %+v", len(want), len(endpoints), endpoints)
	}
	for _, ep := range endpoints {
		key := string(ep.Method) + " " + ep.Path
		if !want[key] {
			t.Errorf("unexpected endpoint %q — dynamic route must not be invented (R2)", key)
		}
	}
}

func TestDiscover_GroupedFixture_ComposesNestedPrefixes(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/echo/grouped")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"main.go"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := map[string]bool{
		"GET /health":        true,
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
			t.Errorf("expected endpoint %q from nested groups, got %+v", k, endpoints)
		}
	}
	if len(endpoints) != len(want) {
		t.Errorf("expected exactly %d endpoints, got %d: %+v", len(want), len(endpoints), endpoints)
	}
}

func TestSkippedDynamicCount_CountsDynamicCalls(t *testing.T) {
	root := os.DirFS("../../../../testdata/echo/simple")
	n, err := SkippedDynamicCount(root, []string{"main.go"})
	if err != nil {
		t.Fatalf("SkippedDynamicCount: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least 1 skipped dynamic call in the simple fixture, got %d", n)
	}
}

func TestExtractRoutes_IgnoresUnknownReceivers(t *testing.T) {
	source := `
e := echo.New()
someLogger.GET("/should-not-count")
e.GET("/should-count", handler)
`
	endpoints := extractRoutes(source)
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint (e.GET only), got %d: %+v", len(endpoints), endpoints)
	}
	if endpoints[0].Path != "/should-count" {
		t.Errorf("Path = %q", endpoints[0].Path)
	}
}
