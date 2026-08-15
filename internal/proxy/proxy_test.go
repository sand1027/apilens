package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/security"
)

// startTestProxy binds a Proxy on an ephemeral loopback port and serves it
// in the background, returning the proxy and a cleanup func.
func startTestProxy(t *testing.T, opts Options) (*Proxy, func()) {
	t.Helper()
	if opts.Bind == "" {
		opts.Bind = "127.0.0.1:0"
	}
	p := New(opts)
	if err := p.Bind(Options{Bind: opts.Bind, AllowRemote: opts.AllowRemote}); err != nil {
		t.Fatalf("Bind: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = p.Serve(ctx)
		close(done)
	}()
	cleanup := func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
		}
	}
	return p, cleanup
}

// clientThroughProxy builds an http.Client that routes every request
// through the given proxy address, as a real forward-proxy-aware client
// (e.g. one configured via HTTP_PROXY) would.
func clientThroughProxy(proxyAddr string) *http.Client {
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddr}
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
}

func TestBind_RejectsNonLoopbackWithoutAllowRemote(t *testing.T) {
	p := New(Options{})
	err := p.Bind(Options{Bind: "0.0.0.0:0"})
	if err == nil {
		t.Fatal("expected bind policy rejection for 0.0.0.0")
	}
}

func TestBind_AllowsLoopback(t *testing.T) {
	p := New(Options{})
	err := p.Bind(Options{Bind: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Bind: %v", err)
	}
	if p.Addr() == "" {
		t.Error("expected Addr() to be set after successful Bind")
	}
}

func TestForward_CapturesAndEmitsExchange(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer upstream.Close()

	p, cleanup := startTestProxy(t, Options{})
	defer cleanup()

	client := clientThroughProxy(p.Addr())
	resp, err := client.Get(upstream.URL + "/health")
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", resp.StatusCode)
	}

	select {
	case ex := <-p.Events():
		if ex.Response.StatusCode != 200 {
			t.Errorf("captured StatusCode = %d, want 200", ex.Response.StatusCode)
		}
		if string(ex.Response.Body) != `{"status":"ok"}` {
			t.Errorf("captured body = %s", ex.Response.Body)
		}
		if ex.Request.Method != "GET" {
			t.Errorf("captured method = %q", ex.Request.Method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a captured exchange")
	}
}

func TestForward_RedactsAuthorizationHeaderInCapture(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	p, cleanup := startTestProxy(t, Options{Redactor: security.New(security.Config{})})
	defer cleanup()

	client := clientThroughProxy(p.Addr())
	req, _ := http.NewRequest("GET", upstream.URL+"/secret", nil)
	req.Header.Set("Authorization", "Bearer real-secret-token")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	resp.Body.Close()

	select {
	case ex := <-p.Events():
		got := ex.Request.Headers.Get("Authorization")
		if got != "Bearer ********" {
			t.Errorf("captured Authorization = %q, want masked", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a captured exchange")
	}
}

func TestForward_UpstreamModeIgnoresRequestHost(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte("target-response"))
	}))
	defer target.Close()

	p, cleanup := startTestProxy(t, Options{Upstream: target.URL})
	defer cleanup()

	// Ask the proxy for a totally different (unreachable) host; --upstream
	// mode should ignore that and forward to `target` anyway.
	client := clientThroughProxy(p.Addr())
	resp, err := client.Get("http://this-host-does-not-exist.invalid/anything")
	if err != nil {
		t.Fatalf("request in upstream mode: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 (from upstream target)", resp.StatusCode)
	}
}

func TestFilter_DropsStaticAssetsByDefault(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	p, cleanup := startTestProxy(t, Options{})
	defer cleanup()

	client := clientThroughProxy(p.Addr())
	resp, err := client.Get(upstream.URL + "/app.js")
	if err != nil {
		t.Fatalf("request through proxy: %v", err)
	}
	resp.Body.Close()

	select {
	case ex := <-p.Events():
		t.Fatalf("expected .js request to be filtered out, got captured exchange %+v", ex)
	case <-time.After(300 * time.Millisecond):
		// no event within a short window — filtering worked
	}
}

func TestFilter_PathPrefixOnlyCapturesMatching(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	p, cleanup := startTestProxy(t, Options{Filter: Filter{PathPrefix: "/api"}})
	defer cleanup()

	client := clientThroughProxy(p.Addr())

	resp, _ := client.Get(upstream.URL + "/other")
	if resp != nil {
		resp.Body.Close()
	}
	resp2, _ := client.Get(upstream.URL + "/api/users")
	if resp2 != nil {
		resp2.Body.Close()
	}

	select {
	case ex := <-p.Events():
		if ex.Request.URL == "" || !containsSubstring(ex.Request.URL, "/api/users") {
			t.Errorf("expected the /api/users request to be captured, got %+v", ex)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the /api/users exchange")
	}

	select {
	case ex := <-p.Events():
		t.Fatalf("expected only one captured exchange (the /api one), got a second: %+v", ex)
	case <-time.After(300 * time.Millisecond):
	}
}

func TestPACScriptProxiesLocalhost(t *testing.T) {
	p, cleanup := startTestProxy(t, Options{Bind: "127.0.0.1:0"})
	defer cleanup()

	resp, err := http.Get("http://" + p.Addr() + "/__apilens__/proxy.pac")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	s := string(body)
	if !containsSubstring(s, "PROXY "+p.Addr()) {
		t.Errorf("PAC missing this proxy: %s", s)
	}
	if !containsSubstring(s, `host === "localhost"`) {
		t.Errorf("PAC should proxy localhost: %s", s)
	}
	if !containsSubstring(s, `host === "::1"`) {
		t.Errorf("PAC should proxy IPv6 loopback: %s", s)
	}
	if !containsSubstring(s, `:4488/*`) || !containsSubstring(s, `DIRECT`) {
		t.Errorf("PAC should skip apilens ui :4488: %s", s)
	}
}

func TestFilter_DropsApiLensDashboard(t *testing.T) {
	p := New(Options{})
	req := httptest.NewRequest("GET", "http://127.0.0.1:4488/api/history?limit=200", nil)
	if p.passesFilter(req) {
		t.Fatal("dashboard polls must not be captured")
	}
	gql := httptest.NewRequest("POST", "http://localhost:3000/graphql", nil)
	if !p.passesFilter(gql) {
		t.Fatal("GraphQL to :3000 must still be captured")
	}
}

func TestBrowserUpdateHostsAreFiltered(t *testing.T) {
	if !isBrowserUpdateHost("update.googleapis.com") {
		t.Fatal("expected googleapis to be noise")
	}
	if isBrowserUpdateHost("localhost:3000") {
		t.Fatal("localhost must not be treated as noise")
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
