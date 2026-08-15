package discovery

import (
	"context"
	"errors"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/sandeepv/apilens/internal/domain"
)

type fakeProvider struct {
	name       string
	detectOK   bool
	detectErr  error
	discoverFn func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error)
}

func (f *fakeProvider) Name() string { return f.name }
func (f *fakeProvider) Detect(ctx context.Context, root fs.FS) (bool, error) {
	return f.detectOK, f.detectErr
}
func (f *fakeProvider) Discover(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
	return f.discoverFn(ctx, root, opts)
}

func ep(method, path string, sources ...string) domain.Endpoint {
	return domain.Endpoint{Method: domain.NormalizeMethod(method), Path: path, Sources: sources, PrimarySource: sources[0]}
}

func TestDiscover_MergesSourcesForSameEndpoint(t *testing.T) {
	openapiP := &fakeProvider{name: "openapi", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		return []domain.Endpoint{ep("GET", "/api/users", "openapi")}, nil
	}}
	expressP := &fakeProvider{name: "express", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		return []domain.Endpoint{ep("GET", "/api/users", "express")}, nil
	}}

	orch := New([]Provider{openapiP, expressP}, nil)
	endpoints, errs, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("unexpected provider errors: %v", errs)
	}
	if len(endpoints) != 1 {
		t.Fatalf("expected 1 merged endpoint, got %d: %+v", len(endpoints), endpoints)
	}
	got := endpoints[0]
	if got.PrimarySource != "openapi" {
		t.Errorf("PrimarySource = %q, want openapi (higher priority)", got.PrimarySource)
	}
	if len(got.Sources) != 2 {
		t.Errorf("Sources = %v, want both openapi and express", got.Sources)
	}
}

func TestDiscover_SkipsDisabledProvider(t *testing.T) {
	called := false
	p := &fakeProvider{name: "express", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		called = true
		return nil, nil
	}}
	orch := New([]Provider{p}, []string{"express"})
	_, _, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if called {
		t.Error("disabled provider's Discover should not be called")
	}
}

func TestDiscover_SkipsUndetectedProvider(t *testing.T) {
	called := false
	p := &fakeProvider{name: "openapi", detectOK: false, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		called = true
		return nil, nil
	}}
	orch := New([]Provider{p}, nil)
	_, _, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if called {
		t.Error("Discover should not be called when Detect returns false")
	}
}

func TestDiscover_ExplicitPathBypassesDetect(t *testing.T) {
	called := false
	p := &fakeProvider{name: "openapi", detectOK: false, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		called = true
		return nil, nil
	}}
	orch := New([]Provider{p}, nil)
	_, _, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{Paths: []string{"openapi.yaml"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !called {
		t.Error("Discover should run when --path is given, even if Detect would say no")
	}
}

func TestDiscover_OneProviderFailureDoesNotAbortOthers(t *testing.T) {
	failing := &fakeProvider{name: "openapi", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		return nil, errors.New("broken spec")
	}}
	working := &fakeProvider{name: "express", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		return []domain.Endpoint{ep("GET", "/health", "express")}, nil
	}}
	orch := New([]Provider{failing, working}, nil)
	endpoints, errs, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(errs) != 1 || errs[0].Provider != "openapi" {
		t.Errorf("expected 1 error from openapi provider, got %+v", errs)
	}
	if len(endpoints) != 1 {
		t.Errorf("expected the working provider's endpoint to survive, got %+v", endpoints)
	}
}

func TestDiscover_SourceFilterLimitsProviders(t *testing.T) {
	called := map[string]bool{}
	openapiP := &fakeProvider{name: "openapi", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		called["openapi"] = true
		return nil, nil
	}}
	expressP := &fakeProvider{name: "express", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		called["express"] = true
		return nil, nil
	}}
	orch := New([]Provider{openapiP, expressP}, nil)
	_, _, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{Enabled: []string{"openapi"}})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if !called["openapi"] || called["express"] {
		t.Errorf("expected only openapi to run, got called=%v", called)
	}
}

func TestDiscover_SortsByPathThenMethod(t *testing.T) {
	p := &fakeProvider{name: "openapi", detectOK: true, discoverFn: func(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error) {
		return []domain.Endpoint{
			ep("POST", "/b", "openapi"),
			ep("GET", "/a", "openapi"),
			ep("GET", "/b", "openapi"),
		}, nil
	}}
	orch := New([]Provider{p}, nil)
	endpoints, _, err := orch.Discover(context.Background(), fstest.MapFS{}, Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	want := []string{"/a GET", "/b GET", "/b POST"}
	for i, w := range want {
		got := endpoints[i].Path + " " + string(endpoints[i].Method)
		if got != w {
			t.Errorf("endpoints[%d] = %q, want %q", i, got, w)
		}
	}
}
