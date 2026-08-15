package mock

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func startTestServer(t *testing.T, endpoints []domain.Endpoint, examples map[domain.EndpointID]domain.Exchange) (*Server, func()) {
	t.Helper()
	s := New(endpoints, examples)
	if err := s.Bind(Options{Bind: "127.0.0.1:0"}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = s.Serve(ctx)
		close(done)
	}()
	// Give Serve a moment to actually start accepting.
	time.Sleep(20 * time.Millisecond)
	return s, func() {
		cancel()
		<-done
	}
}

func TestServer_ReplaysCapturedExample(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users"}}
	ex := domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 200,
			Headers:    http.Header{"Content-Type": []string{"application/json"}},
			Body:       []byte(`{"data":[{"id":1}]}`),
		},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users"): ex,
	}
	s, stop := startTestServer(t, endpoints, examples)
	defer stop()

	resp, err := http.Get("http://" + s.Addr() + "/api/users")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := body["data"]; !ok {
		t.Errorf("expected captured body to be replayed verbatim, got %+v", body)
	}
}

func TestServer_SynthesizesResponseWithoutExample(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/health"}}
	s, stop := startTestServer(t, endpoints, nil)
	defer stop()

	resp, err := http.Get("http://" + s.Addr() + "/health")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200 default", resp.StatusCode)
	}
}

func TestServer_UnknownRouteReturns404(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/health"}}
	s, stop := startTestServer(t, endpoints, nil)
	defer stop()

	resp, err := http.Get("http://" + s.Addr() + "/does-not-exist")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404", resp.StatusCode)
	}
}

func TestServer_MatchesPathParamSegment(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users/:id"}}
	ex := domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 200, Headers: http.Header{}, Body: []byte(`{"id":42}`)},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/:id"): ex,
	}
	s, stop := startTestServer(t, endpoints, examples)
	defer stop()

	resp, err := http.Get("http://" + s.Addr() + "/api/users/42")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestServer_PrefersLiteralRouteOverParamRoute(t *testing.T) {
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/api/users/:id"},
		{Method: "GET", Path: "/api/users/new"},
	}
	literalEx := domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 200, Headers: http.Header{}, Body: []byte(`{"literal":true}`)},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/new"): literalEx,
	}
	s, stop := startTestServer(t, endpoints, examples)
	defer stop()

	resp, err := http.Get("http://" + s.Addr() + "/api/users/new")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if body["literal"] != true {
		t.Errorf("expected the literal route to win over the :id route, got %+v", body)
	}
}

func TestServer_MethodMismatchIsNotFound(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users"}}
	s, stop := startTestServer(t, endpoints, nil)
	defer stop()

	resp, err := http.Post("http://"+s.Addr()+"/api/users", "application/json", nil)
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Errorf("status = %d, want 404 for a method that has no registered route", resp.StatusCode)
	}
}

func TestBind_RejectsNonLoopbackWithoutAllowRemote(t *testing.T) {
	s := New(nil, nil)
	err := s.Bind(Options{Bind: "0.0.0.0:0"})
	if err == nil {
		t.Fatal("expected an error binding a non-loopback address without --allow-remote")
	}
}
