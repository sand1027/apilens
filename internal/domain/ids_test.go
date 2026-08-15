package domain

import "testing"

func TestNormalizePath(t *testing.T) {
	cases := map[string]string{
		"/api/users":       "/api/users",
		"/api/users/":      "/api/users",
		"/":                "/",
		"/api/users/:id":   "/api/users/:id",
		"/api/users/{id}":  "/api/users/:id",
		"/api/users?x=1":   "/api/users",
		"/api/users/{id}/": "/api/users/:id",
		// docs/07-discovery.md section 2: both :name and {name} style
		// params canonicalize to the literal ":id" placeholder,
		// regardless of the original parameter name.
		"/api/:a/nested/{b}": "/api/:id/nested/:id",
	}
	for in, want := range cases {
		if got := NormalizePath(in); got != want {
			t.Errorf("NormalizePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNewEndpointID_SameForEquivalentPaths(t *testing.T) {
	a := NewEndpointID("get", "/api/users/{id}")
	b := NewEndpointID("GET", "/api/users/:id/")
	if a != b {
		t.Errorf("expected same ID for equivalent paths, got %q vs %q", a, b)
	}
}

func TestNewEndpointID_DifferentForDifferentMethods(t *testing.T) {
	a := NewEndpointID("GET", "/api/users")
	b := NewEndpointID("POST", "/api/users")
	if a == b {
		t.Error("expected different IDs for different methods")
	}
}
