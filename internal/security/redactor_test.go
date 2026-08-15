package security

import (
	"net/http"
	"testing"
)

func TestDisplayHeaders_MasksSensitiveHeaders(t *testing.T) {
	r := New(Config{})
	h := http.Header{}
	h.Set("Authorization", "Bearer secret-token-123")
	h.Set("Cookie", "session=abc123")
	h.Set("X-API-Key", "key-xyz")
	h.Set("Content-Type", "application/json")

	out := r.DisplayHeaders(h)

	if got := out.Get("Authorization"); got != "Bearer ********" {
		t.Errorf("Authorization = %q, want %q", got, "Bearer ********")
	}
	if got := out.Get("Cookie"); got != "********" {
		t.Errorf("Cookie = %q, want %q", got, "********")
	}
	if got := out.Get("X-Api-Key"); got != "********" {
		t.Errorf("X-Api-Key = %q, want %q", got, "********")
	}
	if got := out.Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want unchanged", got)
	}
}

func TestDisplayHeaders_BasicAuthMasksButKeepsScheme(t *testing.T) {
	r := New(Config{})
	h := http.Header{}
	h.Set("Authorization", "Basic dXNlcjpwYXNz")

	out := r.DisplayHeaders(h)
	if got := out.Get("Authorization"); got != "Basic ********" {
		t.Errorf("Authorization = %q, want %q", got, "Basic ********")
	}
}

func TestDisplayHeaders_AlwaysMasksEvenWhenCaptureSensitiveHeadersTrue(t *testing.T) {
	// ADR-006: even with capture_sensitive_headers: true, DisplayHeaders
	// must still mask. Only Headers() (in-memory) may return raw values.
	r := New(Config{CaptureSensitiveHeaders: true})
	h := http.Header{}
	h.Set("Authorization", "Bearer secret")

	displayed := r.DisplayHeaders(h)
	if got := displayed.Get("Authorization"); got != "Bearer ********" {
		t.Errorf("DisplayHeaders with capture=true: Authorization = %q, want masked", got)
	}

	raw := r.Headers(h)
	if got := raw.Get("Authorization"); got != "Bearer secret" {
		t.Errorf("Headers() with capture=true: Authorization = %q, want raw value preserved", got)
	}
}

func TestDisplayHeaders_ExtraSensitiveHeadersConfig(t *testing.T) {
	r := New(Config{ExtraSensitiveHeaders: []string{"X-Custom-Token"}})
	h := http.Header{}
	h.Set("X-Custom-Token", "abc")

	out := r.DisplayHeaders(h)
	if got := out.Get("X-Custom-Token"); got != "********" {
		t.Errorf("X-Custom-Token = %q, want masked", got)
	}
}

func TestBody_RedactsSensitiveJSONKeys(t *testing.T) {
	r := New(Config{})
	body := []byte(`{"username":"ada","password":"topsecret","nested":{"token":"xyz"}}`)

	out := r.Body("application/json", body)
	s := string(out)

	if !contains(s, `"password":"********"`) {
		t.Errorf("password not redacted: %s", s)
	}
	if !contains(s, `"token":"********"`) {
		t.Errorf("nested token not redacted: %s", s)
	}
	if !contains(s, `"username":"ada"`) {
		t.Errorf("non-sensitive key should be preserved: %s", s)
	}
}

func TestBody_NonJSONPassesThrough(t *testing.T) {
	r := New(Config{})
	body := []byte("plain text with password=abc123 in it")
	out := r.Body("text/plain", body)
	if string(out) != string(body) {
		t.Errorf("non-JSON body was modified: %s", out)
	}
}

func TestContainsSensitiveBodyKey(t *testing.T) {
	yes := []byte(`{"password":"x"}`)
	no := []byte(`{"name":"x"}`)
	if !ContainsSensitiveBodyKey(yes) {
		t.Error("expected true for body containing 'password'")
	}
	if ContainsSensitiveBodyKey(no) {
		t.Error("expected false for body without sensitive keys")
	}
}

func contains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
