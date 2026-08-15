package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/config"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/history"
	"github.com/sandeepv/apilens/internal/registry"
)

func clientThroughProxy(proxyAddr string) *http.Client {
	proxyURL := &url.URL{Scheme: "http", Host: proxyAddr}
	return &http.Client{
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
		Timeout:   5 * time.Second,
	}
}

func TestWatch_CapturesTrafficAndUpsertsRegistry(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1:0"})
	if err != nil {
		cancel()
		t.Fatalf("Watch: %v", err)
	}

	client := clientThroughProxy(handle.Addr)
	resp, err := client.Get(upstream.URL + "/api/things")
	if err != nil {
		cancel()
		t.Fatalf("request through watch proxy: %v", err)
	}
	resp.Body.Close()

	// Give the event pump a moment to drain the channel into
	// history/registry (it runs in its own goroutine).
	deadline := time.After(2 * time.Second)
	for {
		endpoints := a.Registry.List(registry.Filter{Source: "watch"})
		if len(endpoints) > 0 {
			if endpoints[0].Path != "/api/things" {
				t.Errorf("observed endpoint path = %q", endpoints[0].Path)
			}
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for watch to upsert an observed endpoint")
		case <-time.After(20 * time.Millisecond):
		}
	}

	hist, err := a.HistoryList(0)
	if err != nil {
		t.Fatalf("HistoryList: %v", err)
	}
	if len(hist) == 0 {
		t.Error("expected at least one history entry after watch captured traffic")
	}

	// Stop watch and wait for its background goroutine (and its deferred
	// registry persist) to fully finish before the test's t.TempDir()
	// cleanup runs, otherwise cleanup can race with a concurrent write to
	// .apilens/api/ and fail with "directory not empty".
	cancel()
	for range handle.Events {
	}
}

func TestWatch_PersistsRegistryOnCleanStop(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer upstream.Close()

	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	client := clientThroughProxy(handle.Addr)
	resp, err := client.Get(upstream.URL + "/observed/path")
	if err != nil {
		t.Fatalf("request through watch proxy: %v", err)
	}
	resp.Body.Close()

	// Wait for the event to actually be observed before stopping, so the
	// registry has something to persist.
	deadline := time.After(2 * time.Second)
	for len(a.Registry.List(registry.Filter{Source: "watch"})) == 0 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for watch to observe the endpoint")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel() // clean stop, triggers persistRegistrySnapshot
	// Drain Events until it closes, confirming pumpWatchEvents finished
	// (and therefore its deferred persist ran) before we check the file.
	for range handle.Events {
	}

	a2, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New (second process): %v", err)
	}
	endpoints := a2.Registry.List(registry.Filter{})
	found := false
	for _, ep := range endpoints {
		if ep.Path == "/observed/path" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected the watch-observed endpoint to survive in a new process, got %+v", endpoints)
	}
}

func TestWatch_RejectsNonLoopbackWithoutAllowRemote(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err = a.Watch(ctx, WatchOptions{Bind: "0.0.0.0:0"})
	if err == nil {
		t.Fatal("expected bind policy rejection for 0.0.0.0")
	}
}

func TestWatch_GraphQLBodyUpsertsQueryOperation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"data":{"ping":{"message":"ok"}}}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1:0"})
	if err != nil {
		cancel()
		t.Fatalf("Watch: %v", err)
	}

	client := clientThroughProxy(handle.Addr)
	resp, err := client.Post(upstream.URL+"/graphql", "application/json",
		bytes.NewReader([]byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`)))
	if err != nil {
		cancel()
		t.Fatalf("graphql post through watch: %v", err)
	}
	resp.Body.Close()

	deadline := time.After(2 * time.Second)
	for {
		endpoints := a.Registry.List(registry.Filter{Source: "watch"})
		if len(endpoints) > 0 {
			if endpoints[0].Method != "QUERY" || endpoints[0].Path != "/graphql/query/ping" {
				t.Errorf("observed = %s %s, want QUERY /graphql/query/ping", endpoints[0].Method, endpoints[0].Path)
			}
			tagged := false
			for _, tag := range endpoints[0].Tags {
				if tag == "graphql" {
					tagged = true
				}
			}
			if !tagged {
				t.Errorf("expected graphql tag, got %v", endpoints[0].Tags)
			}
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for GraphQL watch upsert")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	for range handle.Events {
	}
}

func TestHistoryList_SecondProcessReadsProjectSessionFile(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1:0"})
	if err != nil {
		cancel()
		t.Fatalf("Watch: %v", err)
	}

	client := clientThroughProxy(handle.Addr)
	resp, err := client.Get(upstream.URL + "/api/from-watch")
	if err != nil {
		cancel()
		t.Fatalf("request through watch proxy: %v", err)
	}
	resp.Body.Close()

	deadline := time.After(2 * time.Second)
	for {
		hist, err := a.HistoryList(0)
		if err != nil {
			cancel()
			t.Fatalf("HistoryList: %v", err)
		}
		if len(hist) > 0 {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatal("timed out waiting for watch to record history")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	for range handle.Events {
	}

	a2, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New (second process): %v", err)
	}
	hist, err := a2.HistoryList(0)
	if err != nil {
		t.Fatalf("HistoryList: %v", err)
	}
	if len(hist) == 0 {
		t.Fatal("expected history list in a new process to read the project session file")
	}
}

func TestHistoryList_OtherProjectReadsActiveWatchSession(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer upstream.Close()

	apiDir := t.TempDir()
	frontendDir := t.TempDir()
	watchApp, err := New(Options{ProjectDir: apiDir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handle, err := watchApp.Watch(ctx, WatchOptions{Bind: "127.0.0.1:0"})
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}

	client := clientThroughProxy(handle.Addr)
	resp, err := client.Get(upstream.URL + "/graphql")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	resp.Body.Close()

	deadline := time.After(2 * time.Second)
	for {
		hist, err := watchApp.HistoryList(0)
		if err != nil {
			t.Fatalf("HistoryList: %v", err)
		}
		if len(hist) > 0 {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for watch to record history")
		case <-time.After(20 * time.Millisecond):
		}
	}

	uiApp, err := New(Options{ProjectDir: frontendDir})
	if err != nil {
		t.Fatalf("New ui: %v", err)
	}
	hist, err := uiApp.HistoryList(0)
	if err != nil {
		t.Fatalf("ui HistoryList: %v", err)
	}
	if len(hist) == 0 {
		t.Fatalf("ui in a different repo should follow the watch history pointer; active=%q search=%v", history.ActivePath(), history.SearchPaths(frontendDir))
	}
}

func TestHistoryGet_PrefersLatestDuplicateDisplayID(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	sf, err := history.NewSessionFile(history.DefaultPath(a.ProjectDir))
	if err != nil {
		t.Fatalf("NewSessionFile: %v", err)
	}
	if err := sf.Append(domain.Exchange{Display: 1, Request: domain.HTTPRequest{Method: "GET", URL: "http://chrome.example/update"}}); err != nil {
		t.Fatal(err)
	}
	if err := sf.Append(domain.Exchange{Display: 1, Request: domain.HTTPRequest{Method: "POST", URL: "http://localhost:3000/graphql"}}); err != nil {
		t.Fatal(err)
	}

	ex, ok, err := a.HistoryGet(1)
	if err != nil {
		t.Fatalf("HistoryGet: %v", err)
	}
	if !ok {
		t.Fatal("expected to find display id 1 in the session file")
	}
	if ex.Request.URL != "http://localhost:3000/graphql" {
		t.Errorf("HistoryGet(1) URL = %q, want the later GraphQL capture", ex.Request.URL)
	}
}

func TestResolveBind_DefaultIs8888(t *testing.T) {
	got := resolveBind(config.WatchConfig{}, WatchOptions{})
	if got != "127.0.0.1:8888" {
		t.Errorf("resolveBind() = %q, want 127.0.0.1:8888", got)
	}
}

func TestResolveBind_FullAddressIncludingEphemeral(t *testing.T) {
	got := resolveBind(config.WatchConfig{Port: 8888}, WatchOptions{Bind: "127.0.0.1:0"})
	if got != "127.0.0.1:0" {
		t.Errorf("resolveBind(ephemeral) = %q, want 127.0.0.1:0", got)
	}
}

func TestHistoryList_UnreadableSessionFileIsError(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	path := history.DefaultPath(a.ProjectDir)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, err = a.HistoryList(0)
	if err == nil {
		t.Fatal("expected HistoryList to surface a ReadAll error when session.jsonl is a directory")
	}
	if !strings.Contains(err.Error(), "session") && !strings.Contains(err.Error(), "history") {
		t.Errorf("error should mention history/session, got: %v", err)
	}
}

func TestHistoryGet_UnreadableSessionFileIsError(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	path := history.DefaultPath(a.ProjectDir)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err = a.HistoryGet(1)
	if err == nil {
		t.Fatal("expected HistoryGet to surface a ReadAll error")
	}
}
