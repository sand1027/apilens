package express

import (
	"context"
	"os"
	"testing"

	"github.com/sandeepv/apilens/internal/discovery"
)

func TestDetect_TrueWhenPackageJSONHasExpress(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/express/simple")
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Error("expected Detect to find express dependency in package.json")
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

func TestDiscover_SimpleFixture_FindsStaticRoutesOnly(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/express/simple")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"app.js"}})
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

func TestDiscover_MountedFixture_AppliesRouterPrefix(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/express/mounted")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"app.js"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	want := map[string]bool{
		"GET /health":        true,
		"GET /api/users":     true,
		"GET /api/users/:id": false, // path form before normalization is /api/users/:id already
		"POST /api/users":    true,
	}
	found := map[string]bool{}
	for _, ep := range endpoints {
		key := string(ep.Method) + " " + ep.Path
		found[key] = true
	}
	for k, must := range want {
		if must && !found[k] {
			t.Errorf("expected endpoint %q from mounted router, got %+v", k, endpoints)
		}
	}
}

func TestDiscover_NeverInventsDynamicRoutes(t *testing.T) {
	p := New()
	root := os.DirFS("../../../../testdata/express/simple")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"app.js"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	for _, ep := range endpoints {
		if ep.Path == "" {
			t.Error("found an endpoint with an empty path — likely invented from a dynamic call")
		}
	}
}

func TestSkippedDynamicCount_CountsDynamicCalls(t *testing.T) {
	root := os.DirFS("../../../../testdata/express/simple")
	n, err := SkippedDynamicCount(root, []string{"app.js"})
	if err != nil {
		t.Fatalf("SkippedDynamicCount: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least 1 skipped dynamic call in the simple fixture, got %d", n)
	}
}

func TestExtractRoutes_IgnoresNonRouterReceivers(t *testing.T) {
	source := `
const myLogger = require('some-logger');
myLogger.get('/should-not-count');
app.get('/should-count', (req, res) => {});
`
	endpoints := extractRoutes(source)
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 endpoint (app.get only), got %d: %+v", len(endpoints), endpoints)
	}
	if endpoints[0].Path != "/should-count" {
		t.Errorf("Path = %q", endpoints[0].Path)
	}
}
