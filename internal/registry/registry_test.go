package registry

import (
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func ep(method, path string, sources ...string) domain.Endpoint {
	return domain.Endpoint{Method: domain.NormalizeMethod(method), Path: path, Sources: sources}
}

func TestReplace_ThenListReturnsAll(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Replace([]domain.Endpoint{ep("GET", "/a", "openapi"), ep("POST", "/b", "openapi")})
	got := s.List(Filter{})
	if len(got) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(got))
	}
}

func TestReplace_OverwritesPreviousContents(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Replace([]domain.Endpoint{ep("GET", "/old", "openapi")})
	_ = s.Replace([]domain.Endpoint{ep("GET", "/new", "openapi")})
	got := s.List(Filter{})
	if len(got) != 1 || got[0].Path != "/new" {
		t.Errorf("expected only /new to remain, got %+v", got)
	}
}

func TestUpsert_MergesSourcesForExistingEndpoint(t *testing.T) {
	s := NewMemoryStore()
	_ = s.Upsert(ep("GET", "/api/users", "openapi"))
	_ = s.Upsert(ep("GET", "/api/users", "watch"))

	got, ok := s.Get("GET", "/api/users")
	if !ok {
		t.Fatal("expected endpoint to exist")
	}
	if len(got.Sources) != 2 {
		t.Errorf("Sources = %v, want both openapi and watch", got.Sources)
	}
}

func TestUpsert_WatchDoesNotOverwriteOpenAPISpec(t *testing.T) {
	s := NewMemoryStore()
	specEp := ep("GET", "/api/users", "openapi")
	specEp.Spec = &domain.EndpointSpec{Name: "listUsers"}
	specEp.PrimarySource = "openapi"
	_ = s.Upsert(specEp)
	_ = s.Upsert(ep("GET", "/api/users", "watch"))

	got, _ := s.Get("GET", "/api/users")
	if got.Spec == nil || got.Spec.Name != "listUsers" {
		t.Errorf("expected OpenAPI Spec to survive a watch upsert, got %+v", got.Spec)
	}
	if got.PrimarySource != "openapi" {
		t.Errorf("PrimarySource = %q, want openapi to remain primary", got.PrimarySource)
	}
}

func TestList_FiltersByMethodPathTagSource(t *testing.T) {
	s := NewMemoryStore()
	a := ep("GET", "/api/users", "openapi")
	a.Tags = []string{"users"}
	b := ep("POST", "/api/orders", "express")
	b.Tags = []string{"orders"}
	_ = s.Replace([]domain.Endpoint{a, b})

	if got := s.List(Filter{Method: "GET"}); len(got) != 1 || got[0].Path != "/api/users" {
		t.Errorf("Method filter failed: %+v", got)
	}
	if got := s.List(Filter{Path: "orders"}); len(got) != 1 {
		t.Errorf("Path filter failed: %+v", got)
	}
	if got := s.List(Filter{Tag: "orders"}); len(got) != 1 || got[0].Path != "/api/orders" {
		t.Errorf("Tag filter failed: %+v", got)
	}
	if got := s.List(Filter{Source: "express"}); len(got) != 1 || got[0].Path != "/api/orders" {
		t.Errorf("Source filter failed: %+v", got)
	}
}

func TestGetByID_FindsEndpointByComputedID(t *testing.T) {
	s := NewMemoryStore()
	e := ep("GET", "/api/users", "openapi")
	e.ID = domain.NewEndpointID(e.Method, e.Path)
	_ = s.Replace([]domain.Endpoint{e})

	got, ok := s.GetByID(e.ID)
	if !ok || got.Path != "/api/users" {
		t.Errorf("GetByID failed: ok=%v got=%+v", ok, got)
	}
}

func TestSaveYAML_ThenLoadYAML_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/registry.yaml"

	endpoints := []domain.Endpoint{
		ep("GET", "/api/users", "openapi"),
		ep("POST", "/api/users", "openapi"),
	}
	if err := SaveYAML(path, endpoints); err != nil {
		t.Fatalf("SaveYAML: %v", err)
	}

	loaded, err := LoadYAML(path)
	if err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("expected 2 endpoints, got %d: %+v", len(loaded), loaded)
	}
	for _, ep := range loaded {
		if len(ep.Sources) != 1 || ep.Sources[0] != "openapi" {
			t.Errorf("Sources = %v, want [openapi]", ep.Sources)
		}
		if ep.ID == "" {
			t.Error("expected LoadYAML to compute an EndpointID")
		}
	}
}

func TestLoadYAML_MissingFileReturnsNilNoError(t *testing.T) {
	loaded, err := LoadYAML("/does/not/exist/registry.yaml")
	if err != nil {
		t.Fatalf("LoadYAML: %v", err)
	}
	if loaded != nil {
		t.Errorf("expected nil for missing file, got %v", loaded)
	}
}
