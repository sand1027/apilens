package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

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

	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1", Port: 0})
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

	hist := a.HistoryList(0)
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
	handle, err := a.Watch(ctx, WatchOptions{Bind: "127.0.0.1", Port: 0})
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

	_, err = a.Watch(ctx, WatchOptions{Bind: "0.0.0.0", Port: 0})
	if err == nil {
		t.Fatal("expected bind policy rejection for 0.0.0.0")
	}
}
