package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/registry"
)

// newTestApp builds an App rooted at a fresh temp dir, with an OpenAPI
// fixture copied in so discovery has something to find.
func newTestApp(t *testing.T) (*App, string) {
	t.Helper()
	dir := t.TempDir()

	data, err := os.ReadFile("../../testdata/openapi/valid/openapi.yaml")
	if err != nil {
		t.Fatalf("reading fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "openapi.yaml"), data, 0o644); err != nil {
		t.Fatalf("writing fixture: %v", err)
	}

	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, dir
}

func TestDiscover_FindsFixtureEndpointsAndPersistsRegistry(t *testing.T) {
	a, dir := newTestApp(t)

	res, err := a.Discover(context.Background(), DiscoverOptions{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(res.Endpoints) != 3 {
		t.Fatalf("expected 3 endpoints, got %d: %+v", len(res.Endpoints), res.Endpoints)
	}

	registryFile := filepath.Join(dir, ".apilens", "api", "registry.yaml")
	if _, err := os.Stat(registryFile); err != nil {
		t.Errorf("expected registry.yaml to be written: %v", err)
	}
}

func TestDiscover_ThenList_ReturnsSameEndpoints(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	got := a.List(registry.Filter{})
	if len(got) != 3 {
		t.Fatalf("expected 3 endpoints from List, got %d: %+v", len(got), got)
	}
}

func TestList_HydratesFromDiskInNewProcess(t *testing.T) {
	a, dir := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Simulate a new process by building a fresh App over the same dir.
	a2, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New (second process): %v", err)
	}
	endpoints := a2.Registry.List(registry.Filter{})
	if len(endpoints) != 3 {
		t.Fatalf("expected registry to hydrate 3 endpoints from disk, got %d", len(endpoints))
	}
}

func TestInspect_ResolvesByPath(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	insp, err := a.Inspect(context.Background(), InspectRef{Ref: "/api/users", Method: "GET"})
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if insp.Endpoint.Path != "/api/users" {
		t.Errorf("Path = %q", insp.Endpoint.Path)
	}
	if insp.Live != nil {
		t.Error("expected no live probe when Live=false")
	}
}

func TestInspect_AmbiguousPathWithoutMethodReturnsConfigError(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	_, err := a.Inspect(context.Background(), InspectRef{Ref: "/api/users"})
	if err == nil {
		t.Fatal("expected error when /api/users matches both GET and POST without --method")
	}
}

func TestInspect_UnknownRefReturnsNotFound(t *testing.T) {
	a, _ := newTestApp(t)
	_, err := a.Inspect(context.Background(), InspectRef{Ref: "/does/not/exist"})
	if err == nil {
		t.Fatal("expected not-found error for unknown ref")
	}
}
