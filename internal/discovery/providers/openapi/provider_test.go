package openapi

import (
	"context"
	"os"
	"testing"

	"github.com/sandeepv/apilens/internal/discovery"
)

func TestDetect_TrueWhenWellKnownFileExists(t *testing.T) {
	p := New(nil)
	root := os.DirFS("../../../../testdata/openapi/valid")
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if !ok {
		t.Error("expected Detect to find openapi.yaml")
	}
}

func TestDetect_FalseWhenNoSpecPresent(t *testing.T) {
	p := New(nil)
	root := os.DirFS(t.TempDir())
	ok, err := p.Detect(context.Background(), root)
	if err != nil {
		t.Fatalf("Detect: %v", err)
	}
	if ok {
		t.Error("expected Detect to find nothing in an empty dir")
	}
}

func TestDiscover_ParsesOpenAPI3Fixture(t *testing.T) {
	p := New(nil)
	root := os.DirFS("../../../../testdata/openapi/valid")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"openapi.yaml"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(endpoints) != 3 {
		t.Fatalf("expected 3 endpoints (GET/POST /api/users, GET /api/users/{id}), got %d: %+v", len(endpoints), endpoints)
	}
	for _, ep := range endpoints {
		if ep.PrimarySource != "openapi" {
			t.Errorf("PrimarySource = %q, want openapi", ep.PrimarySource)
		}
		if len(ep.Sources) != 1 || ep.Sources[0] != "openapi" {
			t.Errorf("Sources = %v, want [openapi]", ep.Sources)
		}
	}
}

func TestDiscover_ParsesSwagger2Fixture(t *testing.T) {
	p := New(nil)
	root := os.DirFS("../../../../testdata/openapi/valid")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"swagger.json"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints from swagger2.json, got %d: %+v", len(endpoints), endpoints)
	}
}

func TestDiscover_BrokenSpecWithExplicitPathReturnsError(t *testing.T) {
	p := New(nil)
	root := os.DirFS("../../../../testdata/openapi/invalid")
	_, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"broken.yaml"}})
	if err == nil {
		t.Fatal("expected error for explicitly requested broken spec")
	}
}

func TestDiscover_BrokenSpecViaWalkIsSkippedNotFatal(t *testing.T) {
	// Mix a valid and a broken spec in the same directory, discovered via
	// well-known filename lookup (no explicit --path): the broken one
	// must be skipped, not abort the whole call
	// (docs/11-risks-and-gaps.md R1).
	dir := t.TempDir()
	if err := os.WriteFile(dir+"/openapi.yaml", mustRead(t, "../../../../testdata/openapi/invalid/broken.yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	p := New(nil)
	root := os.DirFS(dir)
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{})
	if err != nil {
		t.Fatalf("Discover should not error on a non-explicit broken spec: %v", err)
	}
	if len(endpoints) != 0 {
		t.Errorf("expected 0 endpoints from a broken spec, got %+v", endpoints)
	}
}

func TestDiscover_EndpointHasOperationIDAsSpecName(t *testing.T) {
	p := New(nil)
	root := os.DirFS("../../../../testdata/openapi/valid")
	endpoints, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"openapi.yaml"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	found := false
	for _, ep := range endpoints {
		if ep.Spec != nil && ep.Spec.Name == "listUsers" {
			found = true
		}
	}
	if !found {
		t.Error("expected an endpoint with Spec.Name == \"listUsers\"")
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return data
}
