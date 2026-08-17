package testrunner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/assertions"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/runner"
	"github.com/sandeepv/apilens/internal/security"
)

func TestRun_CaptureResponseOffByDefault(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"user":{"id":1}}}`))
	}))
	defer ts.Close()

	env := newTestEnv(t, ts)
	// No WithCaptureResponse option -- this is the default every existing
	// caller of testrunner.New already uses.
	tr := New(runner.New(), env, assertions.New(), nil)
	tc := statusEqualsTest("t1", "GET", "{{base_url}}/x", 200)

	report, err := tr.Run(context.Background(), []domain.TestCase{tc}, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if r.ResponseBody != nil {
		t.Errorf("expected no captured response body without WithCaptureResponse, got %q", r.ResponseBody)
	}
	if r.ResponseHeaders != nil {
		t.Errorf("expected no captured response headers without WithCaptureResponse, got %v", r.ResponseHeaders)
	}
}

func TestRun_CaptureResponseIncludesBodyAndHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Request-Id", "abc-123")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"data":{"user":{"id":1,"name":"Ada"}}}`))
	}))
	defer ts.Close()

	env := newTestEnv(t, ts)
	tr := New(runner.New(), env, assertions.New(), nil, WithCaptureResponse(security.New(security.Config{})))
	tc := statusEqualsTest("t1", "GET", "{{base_url}}/x", 200)

	report, err := tr.Run(context.Background(), []domain.TestCase{tc}, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if string(r.ResponseBody) != `{"data":{"user":{"id":1,"name":"Ada"}}}` {
		t.Errorf("ResponseBody = %q", r.ResponseBody)
	}
	if got := r.ResponseHeaders["X-Request-Id"]; len(got) != 1 || got[0] != "abc-123" {
		t.Errorf("ResponseHeaders[X-Request-Id] = %v", got)
	}
}

func TestRun_CaptureResponseRedactsSensitiveBodyKeys(t *testing.T) {
	// The whole point of routing capture through internal/security.Redactor
	// rather than attaching the raw exchange body directly: a captured
	// report must apply the SAME masking every other display/report path
	// in this codebase already uses (docs/09-security.md, ADR-006).
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"user":{"id":1},"access_token":"super-secret-value","password":"hunter2"}`))
	}))
	defer ts.Close()

	env := newTestEnv(t, ts)
	tr := New(runner.New(), env, assertions.New(), nil, WithCaptureResponse(security.New(security.Config{})))
	tc := statusEqualsTest("t1", "GET", "{{base_url}}/x", 200)

	report, err := tr.Run(context.Background(), []domain.TestCase{tc}, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if containsSubstring(string(r.ResponseBody), "super-secret-value") {
		t.Errorf("captured response body leaked a raw access_token, got %s", r.ResponseBody)
	}
	if containsSubstring(string(r.ResponseBody), "hunter2") {
		t.Errorf("captured response body leaked a raw password, got %s", r.ResponseBody)
	}
	if !containsSubstring(string(r.ResponseBody), "********") {
		t.Errorf("expected masked values in captured body, got %s", r.ResponseBody)
	}
	// The non-sensitive field must survive untouched.
	if !containsSubstring(string(r.ResponseBody), `"id":1`) {
		t.Errorf("expected non-sensitive fields to survive redaction, got %s", r.ResponseBody)
	}
}

func TestRun_CaptureResponseRedactsSensitiveHeaders(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Set-Cookie", "session=raw-secret-cookie-value")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer ts.Close()

	env := newTestEnv(t, ts)
	tr := New(runner.New(), env, assertions.New(), nil, WithCaptureResponse(security.New(security.Config{})))
	tc := statusEqualsTest("t1", "GET", "{{base_url}}/x", 200)

	report, err := tr.Run(context.Background(), []domain.TestCase{tc}, Options{Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	cookie := r.ResponseHeaders["Set-Cookie"]
	if len(cookie) == 0 {
		t.Fatal("expected a Set-Cookie header to be present (masked)")
	}
	if containsSubstring(cookie[0], "raw-secret-cookie-value") {
		t.Errorf("Set-Cookie header leaked its raw value: %v", cookie)
	}
}

func TestRun_CaptureResponseNotAppliedOnTransportError(t *testing.T) {
	// A true transport error (server unreachable) never produces a real
	// response to capture -- confirm this doesn't panic and simply leaves
	// ResponseBody/Headers empty.
	env := newTestEnv(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})))
	tr := New(runner.New(), env, assertions.New(), nil, WithCaptureResponse(security.New(security.Config{})))
	tc := statusEqualsTest("t1", "GET", "http://127.0.0.1:1/unreachable", 200)
	tc.Retries = 0

	report, err := tr.Run(context.Background(), []domain.TestCase{tc}, Options{Timeout: 500 * time.Millisecond})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	r := report.Results[0]
	if r.Status != domain.StatusErrored {
		t.Fatalf("expected StatusErrored for an unreachable host, got %s", r.Status)
	}
	if r.ResponseBody != nil {
		t.Errorf("expected no captured body for a transport error, got %q", r.ResponseBody)
	}
}

func containsSubstring(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
