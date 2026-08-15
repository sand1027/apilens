package runner

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestDo_SuccessfulRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test", "1")
		w.WriteHeader(201)
		w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	r := New()
	ex, err := r.Do(context.Background(), domain.HTTPRequest{
		Method: "GET", URL: ts.URL, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if ex.Response.StatusCode != 201 {
		t.Errorf("StatusCode = %d, want 201", ex.Response.StatusCode)
	}
	if string(ex.Response.Body) != `{"ok":true}` {
		t.Errorf("Body = %s", ex.Response.Body)
	}
	if ex.Response.Headers.Get("X-Test") != "1" {
		t.Errorf("X-Test header missing")
	}
	if ex.Timing.Duration <= 0 {
		t.Errorf("Duration should be > 0")
	}
}

func TestDo_TransportErrorWrapsErrTransport(t *testing.T) {
	r := New()
	_, err := r.Do(context.Background(), domain.HTTPRequest{
		Method: "GET", URL: "http://127.0.0.1:1/unreachable", Timeout: 300 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected transport error")
	}
	if !errors.Is(err, domain.ErrTransport) {
		t.Errorf("error %v does not wrap domain.ErrTransport", err)
	}
}

func TestDo_ResponseTruncatedAtMaxSize(t *testing.T) {
	body := strings.Repeat("a", 100)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
	defer ts.Close()

	r := New(WithMaxResponseSize(10))
	ex, err := r.Do(context.Background(), domain.HTTPRequest{
		Method: "GET", URL: ts.URL, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if !ex.Response.Truncated {
		t.Error("expected Truncated = true")
	}
	if len(ex.Response.Body) != 10 {
		t.Errorf("Body length = %d, want 10", len(ex.Response.Body))
	}
}

func TestDo_QueryParamsMergedIntoURL(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		w.WriteHeader(200)
	}))
	defer ts.Close()

	r := New()
	query := url.Values{"include": {"profile"}}
	_, err := r.Do(context.Background(), domain.HTTPRequest{
		Method: "GET", URL: ts.URL, Query: query, Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if gotQuery != "include=profile" {
		t.Errorf("query = %q, want %q", gotQuery, "include=profile")
	}
}
