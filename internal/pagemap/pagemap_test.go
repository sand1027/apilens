package pagemap

import (
	"net/http"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func exWithReferer(method, url, referer string) domain.Exchange {
	h := http.Header{}
	if referer != "" {
		h.Set("Referer", referer)
	}
	return domain.Exchange{Request: domain.HTTPRequest{Method: domain.Method(method), URL: url, Headers: h}}
}

func TestBuild_GroupsCallsByRefererPage(t *testing.T) {
	exchanges := []domain.Exchange{
		exWithReferer("GET", "http://api.x/api/users", "http://app.x/dashboard"),
		exWithReferer("GET", "http://api.x/api/orders", "http://app.x/dashboard"),
		exWithReferer("GET", "http://api.x/api/users", "http://app.x/profile"),
	}
	mappings := Build(exchanges)
	if len(mappings) != 2 {
		t.Fatalf("expected 2 distinct pages, got %d: %+v", len(mappings), mappings)
	}

	var dashboard, profile *PageMapping
	for i := range mappings {
		switch mappings[i].Page {
		case "/dashboard":
			dashboard = &mappings[i]
		case "/profile":
			profile = &mappings[i]
		}
	}
	if dashboard == nil || len(dashboard.Calls) != 2 {
		t.Fatalf("expected /dashboard to have 2 distinct calls, got %+v", dashboard)
	}
	if profile == nil || len(profile.Calls) != 1 {
		t.Fatalf("expected /profile to have 1 distinct call, got %+v", profile)
	}
}

func TestBuild_CountsRepeatedCallsToSamePage(t *testing.T) {
	exchanges := []domain.Exchange{
		exWithReferer("GET", "http://api.x/api/users", "http://app.x/dashboard"),
		exWithReferer("GET", "http://api.x/api/users", "http://app.x/dashboard"),
	}
	mappings := Build(exchanges)
	if len(mappings) != 1 || len(mappings[0].Calls) != 1 {
		t.Fatalf("expected 1 page with 1 distinct call, got %+v", mappings)
	}
	if mappings[0].Calls[0].Count != 2 {
		t.Errorf("expected count 2 for the repeated call, got %d", mappings[0].Calls[0].Count)
	}
}

func TestBuild_MissingRefererGroupsUnderEmptyPageKey(t *testing.T) {
	exchanges := []domain.Exchange{
		exWithReferer("GET", "http://api.x/api/users", ""),
	}
	mappings := Build(exchanges)
	if len(mappings) != 1 || mappings[0].Page != "" {
		t.Fatalf("expected a single mapping under the empty page key, got %+v", mappings)
	}
}

func TestBuild_EmptyInputProducesEmptySlice(t *testing.T) {
	mappings := Build(nil)
	if len(mappings) != 0 {
		t.Errorf("expected no mappings for empty input, got %+v", mappings)
	}
}

func TestBuild_NormalizesCallPath(t *testing.T) {
	exchanges := []domain.Exchange{
		exWithReferer("GET", "http://api.x/api/users/42", "http://app.x/dashboard"),
		exWithReferer("GET", "http://api.x/api/users/99", "http://app.x/dashboard"),
	}
	mappings := Build(exchanges)
	if len(mappings) != 1 {
		t.Fatalf("expected 1 page, got %+v", mappings)
	}
	// Note: /api/users/42 and /api/users/99 are literal (not :id-style)
	// paths, so NormalizePath does NOT collapse them — this test
	// documents that behavior rather than assuming clustering.
	if len(mappings[0].Calls) != 2 {
		t.Errorf("expected 2 distinct literal-path calls (no :id clustering), got %+v", mappings[0].Calls)
	}
}
