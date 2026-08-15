package environment

import (
	"net/http"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

// newTestResolver builds a Resolver with fake OS env lookup, so tests don't
// depend on the actual process environment.
func newTestResolver(osEnv map[string]string) *Resolver {
	r := New()
	r.osEnv = func(key string) (string, bool) {
		v, ok := osEnv[key]
		return v, ok
	}
	return r
}

func TestLoadDir_DoesNotExpandOSEnvEagerly(t *testing.T) {
	// LoadDir must succeed even when a referenced OS variable (AUTH_TOKEN)
	// is not set — expansion is deferred to Interpolate, so that commands
	// which never interpolate (discover, list, inspect without --live,
	// env list) are not blocked by an unrelated missing secret.
	r := newTestResolver(map[string]string{}) // AUTH_TOKEN not set
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir should succeed without AUTH_TOKEN set: %v", err)
	}
	env, ok := r.Get("local")
	if !ok {
		t.Fatal("expected environment \"local\" to be loaded")
	}
	if env.Variables["token"] != "${AUTH_TOKEN}" {
		t.Errorf("token = %q, want the raw placeholder to survive LoadDir", env.Variables["token"])
	}
	if env.BaseURL != "http://localhost:5050" {
		t.Errorf("base_url = %q", env.BaseURL)
	}
}

func TestInterpolate_ExpandsOSEnvFromVariable(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "secret123"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if err := r.Use("local"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	got, err := r.Interpolate("{{token}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got != "secret123" {
		t.Errorf("Interpolate(\"{{token}}\") = %q, want %q", got, "secret123")
	}
}

func TestInterpolate_MissingOSEnvIsFailClosed(t *testing.T) {
	// ADR-015: a missing ${ENV_VAR} backing a variable actually used by
	// Interpolate must error, not silently substitute an empty string.
	// This now fires at interpolation time rather than load time.
	r := newTestResolver(map[string]string{}) // AUTH_TOKEN not set
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if err := r.Use("local"); err != nil {
		t.Fatalf("Use: %v", err)
	}
	_, err := r.Interpolate("{{token}}")
	if err == nil {
		t.Fatal("expected error interpolating {{token}} without AUTH_TOKEN set")
	}
}

func TestLoadDir_OSEnvOverlaysFileValue(t *testing.T) {
	// docs/06-test-dsl.md section 7: OS env overlays file values for the
	// same variable key.
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "from-env-expansion", "token": "overlay-value"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	env, _ := r.Get("local")
	if env.Variables["token"] != "overlay-value" {
		t.Errorf("token = %q, want OS env overlay %q", env.Variables["token"], "overlay-value")
	}
}

func TestInterpolate_UndefinedVariableFailsClosed(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	_ = r.Use("local")

	_, err := r.Interpolate("{{undefined_var}}")
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
}

func TestInterpolate_BaseURLAndVariables(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	if err := r.Use("local"); err != nil {
		t.Fatalf("Use: %v", err)
	}

	got, err := r.Interpolate("{{base_url}}/api/users/{{user_id}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	want := "http://localhost:5050/api/users/1"
	if got != want {
		t.Errorf("Interpolate = %q, want %q", got, want)
	}
}

func TestUse_UnknownEnvironmentReturnsNotFound(t *testing.T) {
	r := newTestResolver(map[string]string{})
	err := r.Use("does-not-exist")
	if err == nil {
		t.Fatal("expected error for unknown environment")
	}
}

func TestInterpolateRequest_AppliesBearerAuth(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "tok123"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	_ = r.Use("local")

	req, err := r.InterpolateRequest(domain.RequestTemplate{
		Method: domain.NormalizeMethod("GET"),
		URL:    "{{base_url}}/health",
		Auth:   &domain.AuthTemplate{Type: "bearer", Token: "{{token}}"},
	})
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer tok123" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer tok123")
	}
	if req.URL != "http://localhost:5050/health" {
		t.Errorf("URL = %q", req.URL)
	}
}

func TestInterpolateRequest_JSONBodyInterpolatesStringLeaves(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	_ = r.Use("local")

	req, err := r.InterpolateRequest(domain.RequestTemplate{
		Method: domain.NormalizeMethod("POST"),
		URL:    "{{base_url}}/api/users",
		Body: domain.BodyTemplate{
			JSON: map[string]any{"id": "{{user_id}}", "name": "static"},
		},
	})
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	if req.Headers.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q", req.Headers.Get("Content-Type"))
	}
	body := string(req.Body)
	if !containsStr(body, `"id":"1"`) {
		t.Errorf("body = %s, want id interpolated to 1", body)
	}
}

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// --- DSL v2 chaining (plan.md v9): responses.<id>.* lookups ---

func TestInterpolate_ResponsesStatus(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	if err := r.LoadDir("../../testdata/environments/valid"); err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	_ = r.Use("local")

	r.RecordResponse("login", domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 201},
	})

	got, err := r.Interpolate("{{responses.login.status}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got != "201" {
		t.Errorf("got %q, want %q", got, "201")
	}
}

func TestInterpolate_ResponsesHeader(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	h := http.Header{}
	h.Set("X-Request-Id", "abc-123")
	r.RecordResponse("login", domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 200, Headers: h},
	})

	got, err := r.Interpolate("{{responses.login.headers.x-request-id}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got != "abc-123" {
		t.Errorf("got %q, want %q", got, "abc-123")
	}
}

func TestInterpolate_ResponsesBodyJSONPath(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	r.RecordResponse("login", domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 200,
			Body:       []byte(`{"data":{"token":"tok-xyz","id":42}}`),
		},
	})

	got, err := r.Interpolate("Bearer {{responses.login.body.data.token}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got != "Bearer tok-xyz" {
		t.Errorf("got %q", got)
	}

	got2, err := r.Interpolate("{{responses.login.body.data.id}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got2 != "42" {
		t.Errorf("got %q, want %q", got2, "42")
	}
}

func TestInterpolate_ResponsesRawBody(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	r.RecordResponse("ping", domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte("pong")},
	})

	got, err := r.Interpolate("{{responses.ping.body}}")
	if err != nil {
		t.Fatalf("Interpolate: %v", err)
	}
	if got != "pong" {
		t.Errorf("got %q, want %q", got, "pong")
	}
}

func TestInterpolate_UnknownResponseIDFailsClosed(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	_, err := r.Interpolate("{{responses.never-ran.status}}")
	if err == nil {
		t.Fatal("expected error referencing a response that was never recorded")
	}
}

func TestInterpolate_MissingJSONPathInResponseBodyFailsClosed(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	r.RecordResponse("login", domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{}}`)},
	})

	_, err := r.Interpolate("{{responses.login.body.data.token}}")
	if err == nil {
		t.Fatal("expected error for a JSON path that doesn't exist in the recorded response")
	}
}

func TestResetResponses_ClearsRecordedResponses(t *testing.T) {
	r := newTestResolver(map[string]string{"AUTH_TOKEN": "x"})
	_ = r.LoadDir("../../testdata/environments/valid")
	_ = r.Use("local")

	r.RecordResponse("login", domain.Exchange{Response: domain.HTTPResponse{StatusCode: 200}})
	r.ResetResponses()

	_, err := r.Interpolate("{{responses.login.status}}")
	if err == nil {
		t.Fatal("expected error after ResetResponses cleared the recorded response")
	}
}
