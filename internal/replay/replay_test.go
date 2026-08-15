package replay

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/runner"
)

func storedExchange(url, method string, headers http.Header) domain.Exchange {
	return domain.Exchange{
		Display: 1,
		Request: domain.HTTPRequest{
			Method:  domain.NormalizeMethod(method),
			URL:     url,
			Headers: headers,
		},
	}
}

func TestReplay_ExecutesReconstructedRequest(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	stored := storedExchange(ts.URL+"/original", "GET", http.Header{})
	ex, err := svc.Replay(context.Background(), stored, Overrides{})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotMethod != "GET" || gotPath != "/original" {
		t.Errorf("got %s %s, want GET /original", gotMethod, gotPath)
	}
	if ex.Response.StatusCode != 200 {
		t.Errorf("StatusCode = %d", ex.Response.StatusCode)
	}
}

func TestReplay_AppliesMethodOverride(t *testing.T) {
	var gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	stored := storedExchange(ts.URL+"/x", "GET", http.Header{})
	method := "POST"
	_, err := svc.Replay(context.Background(), stored, Overrides{Method: &method})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotMethod != "POST" {
		t.Errorf("gotMethod = %q, want POST", gotMethod)
	}
}

func TestReplay_AppliesURLOverride(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	stored := storedExchange("http://example.invalid/old", "GET", http.Header{})
	newURL := ts.URL + "/new"
	_, err := svc.Replay(context.Background(), stored, Overrides{URL: &newURL})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotPath != "/new" {
		t.Errorf("gotPath = %q, want /new", gotPath)
	}
}

func TestReplay_AppliesHeaderSetAndUnset(t *testing.T) {
	var gotHeaders http.Header
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	h := http.Header{}
	h.Set("X-Keep", "yes")
	h.Set("X-Remove", "should-go")
	stored := storedExchange(ts.URL+"/x", "GET", h)

	_, err := svc.Replay(context.Background(), stored, Overrides{
		Headers: map[string]string{"X-New": "added"},
		Unset:   []string{"X-Remove"},
	})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotHeaders.Get("X-Keep") != "yes" {
		t.Errorf("X-Keep = %q, want preserved", gotHeaders.Get("X-Keep"))
	}
	if gotHeaders.Get("X-Remove") != "" {
		t.Errorf("X-Remove = %q, want removed", gotHeaders.Get("X-Remove"))
	}
	if gotHeaders.Get("X-New") != "added" {
		t.Errorf("X-New = %q, want added", gotHeaders.Get("X-New"))
	}
}

func TestReplay_AppliesQueryOverride(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	stored := storedExchange(ts.URL+"/x?page=1", "GET", http.Header{})
	_, err := svc.Replay(context.Background(), stored, Overrides{Query: map[string]string{"page": "2"}})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if gotQuery != "page=2" {
		t.Errorf("gotQuery = %q, want page=2", gotQuery)
	}
}

func TestReplay_AppliesBodyOverride(t *testing.T) {
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 1024)
		n, _ := r.Body.Read(buf)
		gotBody = buf[:n]
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	stored := storedExchange(ts.URL+"/x", "POST", http.Header{})
	stored.Request.Body = []byte(`{"old":true}`)
	_, err := svc.Replay(context.Background(), stored, Overrides{Body: []byte(`{"new":true}`)})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if string(gotBody) != `{"new":true}` {
		t.Errorf("gotBody = %s, want {\"new\":true}", gotBody)
	}
}

func TestReplay_RefusesMaskedAuthorizationHeader(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	h := http.Header{}
	h.Set("Authorization", "Bearer ********")
	stored := storedExchange(ts.URL+"/x", "GET", h)

	_, err := svc.Replay(context.Background(), stored, Overrides{})
	if err == nil {
		t.Fatal("expected error replaying a masked Authorization header")
	}
}

func TestReplay_AllowsReplayWhenAuthOverridden(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	svc := New(runner.New())
	h := http.Header{}
	h.Set("Authorization", "Bearer ********")
	stored := storedExchange(ts.URL+"/x", "GET", h)

	_, err := svc.Replay(context.Background(), stored, Overrides{
		Headers: map[string]string{"Authorization": "Bearer real-token"},
	})
	if err != nil {
		t.Fatalf("expected replay to succeed once Authorization is overridden: %v", err)
	}
}
