package discovery

import (
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestApplyFilters_NoOptionsReturnsInputUnchanged(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users"}}
	out := ApplyFilters(endpoints, FilterOptions{})
	if len(out) != 1 || out[0].Path != "/api/users" {
		t.Errorf("expected passthrough, got %+v", out)
	}
}

func TestApplyFilters_IgnoreDropsMatchingEndpoints(t *testing.T) {
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/internal/debug"},
		{Method: "GET", Path: "/api/users"},
	}
	out := ApplyFilters(endpoints, FilterOptions{Ignore: []string{"/internal/*"}})
	if len(out) != 1 {
		t.Fatalf("expected 1 endpoint after ignore, got %d: %+v", len(out), out)
	}
	if out[0].Path != "/api/users" {
		t.Errorf("expected /api/users to survive, got %+v", out[0])
	}
}

func TestApplyFilters_TagAppendsMatchingTag(t *testing.T) {
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/api/users"},
		{Method: "GET", Path: "/api/orders"},
	}
	out := ApplyFilters(endpoints, FilterOptions{
		Tag: map[string]string{"/api/users*": "users-suite"},
	})
	var usersTags, ordersTags []string
	for _, ep := range out {
		if ep.Path == "/api/users" {
			usersTags = ep.Tags
		}
		if ep.Path == "/api/orders" {
			ordersTags = ep.Tags
		}
	}
	if len(usersTags) != 1 || usersTags[0] != "users-suite" {
		t.Errorf("expected /api/users tagged users-suite, got %v", usersTags)
	}
	if len(ordersTags) != 0 {
		t.Errorf("expected /api/orders untagged, got %v", ordersTags)
	}
}

func TestApplyFilters_TagDoesNotDuplicateExistingTag(t *testing.T) {
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/api/users", Tags: []string{"users-suite"}},
	}
	out := ApplyFilters(endpoints, FilterOptions{
		Tag: map[string]string{"/api/*": "users-suite"},
	})
	if len(out[0].Tags) != 1 {
		t.Errorf("expected no duplicate tag, got %v", out[0].Tags)
	}
}

func TestApplyFilters_IgnoreWinsOverTag(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/internal/debug"}}
	out := ApplyFilters(endpoints, FilterOptions{
		Ignore: []string{"/internal/*"},
		Tag:    map[string]string{"/internal/*": "debug"},
	})
	if len(out) != 0 {
		t.Errorf("expected ignored endpoint to stay dropped even with a matching tag pattern, got %+v", out)
	}
}

func TestApplyFilters_DoesNotMutateInputSlice(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users"}}
	_ = ApplyFilters(endpoints, FilterOptions{Tag: map[string]string{"/api/*": "x"}})
	if len(endpoints[0].Tags) != 0 {
		t.Errorf("expected original slice's Endpoint to be unmodified, got %+v", endpoints[0])
	}
}
