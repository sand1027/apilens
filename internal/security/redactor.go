// Package security implements the redaction and bind-policy rules described
// in docs/09-security.md. It is mandatory on every display, persist, and
// generate path — not an optional reporter feature.
package security

import (
	"encoding/json"
	"net/http"
	"strings"
)

const maskValue = "********"

// defaultSensitiveHeaders is always masked in display, regardless of
// capture_sensitive_headers (docs/09-security.md section 2).
var defaultSensitiveHeaders = map[string]struct{}{
	"authorization":       {},
	"proxy-authorization": {},
	"cookie":              {},
	"set-cookie":          {},
	"x-api-key":           {},
	"api-key":             {},
	"x-api-token":         {},
	"x-auth-token":        {},
	"x-access-token":      {},
	"x-csrf-token":        {},
	"x-csrftoken":         {},
	"x-session-id":        {},
}

// defaultSensitiveBodyKeys are JSON object keys treated as secrets in
// bodies and query parameters (docs/09-security.md section 2).
var defaultSensitiveBodyKeys = map[string]struct{}{
	"password":      {},
	"passwd":        {},
	"secret":        {},
	"token":         {},
	"access_token":  {},
	"refresh_token": {},
	"id_token":      {},
	"api_key":       {},
	"apikey":        {},
	"client_secret": {},
	"authorization": {},
	"credit_card":   {},
	"ssn":           {},
}

// Config configures a Redactor. CaptureSensitiveHeaders and ExtraHeaders map
// to `security.capture_sensitive_headers` / `security.sensitive_headers` in
// config.yaml. Even when CaptureSensitiveHeaders is true, DisplayHeaders
// still masks — only the in-memory Headers() copy stays raw (ADR-006).
type Config struct {
	CaptureSensitiveHeaders bool
	ExtraSensitiveHeaders   []string
	MaxResponseSize         int64
}

// DefaultMaxResponseSize is 5MB, per docs/09-security.md section 4/10.
const DefaultMaxResponseSize = 5 * 1024 * 1024

// Redactor implements the docs/04-interfaces.md section 13 contract.
type Redactor struct {
	cfg            Config
	extraHeaderSet map[string]struct{}
}

// New builds a Redactor from Config. Header names in ExtraSensitiveHeaders
// are matched case-insensitively.
func New(cfg Config) *Redactor {
	extra := make(map[string]struct{}, len(cfg.ExtraSensitiveHeaders))
	for _, h := range cfg.ExtraSensitiveHeaders {
		extra[strings.ToLower(h)] = struct{}{}
	}
	return &Redactor{cfg: cfg, extraHeaderSet: extra}
}

func (r *Redactor) isSensitiveHeader(name string) bool {
	lower := strings.ToLower(name)
	if _, ok := defaultSensitiveHeaders[lower]; ok {
		return true
	}
	_, ok := r.extraHeaderSet[lower]
	return ok
}

// maskHeaderValue preserves the auth scheme name (e.g. "Bearer", "Basic")
// but masks the credential itself, per docs/09-security.md section 4.
func maskHeaderValue(name, value string) string {
	lower := strings.ToLower(name)
	if lower == "authorization" || lower == "proxy-authorization" {
		parts := strings.SplitN(value, " ", 2)
		if len(parts) == 2 && parts[0] != "" {
			return parts[0] + " " + maskValue
		}
	}
	return maskValue
}

// Headers returns a copy of h with sensitive header values masked, unless
// CaptureSensitiveHeaders is true — in which case the raw copy is returned
// for in-memory use only. Callers that display or persist to disk must use
// DisplayHeaders instead, which always masks (ADR-006).
func (r *Redactor) Headers(h http.Header) http.Header {
	if r.cfg.CaptureSensitiveHeaders {
		return cloneHeader(h)
	}
	return r.DisplayHeaders(h)
}

// DisplayHeaders always masks sensitive headers, regardless of config. This
// is what reporters, JSON output, and generated tests must use.
func (r *Redactor) DisplayHeaders(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for name, values := range h {
		if r.isSensitiveHeader(name) {
			masked := make([]string, len(values))
			for i, v := range values {
				masked[i] = maskHeaderValue(name, v)
			}
			out[name] = masked
			continue
		}
		out[name] = append([]string(nil), values...)
	}
	return out
}

func cloneHeader(h http.Header) http.Header {
	out := make(http.Header, len(h))
	for k, v := range h {
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Body redacts sensitive JSON object keys in a body. Non-JSON bodies (or
// bodies that fail to parse) are returned unchanged — redaction only
// understands structured JSON per docs/09-security.md section 2.
func (r *Redactor) Body(contentType string, body []byte) []byte {
	if len(body) == 0 {
		return body
	}
	if !strings.Contains(strings.ToLower(contentType), "json") {
		return body
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}
	redacted := redactValue(parsed)
	out, err := json.Marshal(redacted)
	if err != nil {
		return body
	}
	return out
}

func redactValue(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			if _, sensitive := defaultSensitiveBodyKeys[strings.ToLower(k)]; sensitive {
				out[k] = maskValue
				continue
			}
			out[k] = redactValue(val)
		}
		return out
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			out[i] = redactValue(val)
		}
		return out
	default:
		return t
	}
}

// ContainsSensitiveBodyKey reports whether a JSON body (or object) contains
// any key from the sensitive body-key list. Used by generate to decide
// whether to drop a body entirely (docs/06-test-dsl.md section 10).
func ContainsSensitiveBodyKey(body []byte) bool {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return false
	}
	return containsSensitiveKey(parsed)
}

func containsSensitiveKey(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			if _, sensitive := defaultSensitiveBodyKeys[strings.ToLower(k)]; sensitive {
				return true
			}
			if containsSensitiveKey(val) {
				return true
			}
		}
	case []any:
		for _, val := range t {
			if containsSensitiveKey(val) {
				return true
			}
		}
	}
	return false
}

// RedactQueryValues masks sensitive query parameter values in a raw query
// string representation (used by display paths, not by the runner itself).
func RedactQueryKeys(values map[string][]string) map[string][]string {
	out := make(map[string][]string, len(values))
	for k, v := range values {
		if _, sensitive := defaultSensitiveBodyKeys[strings.ToLower(k)]; sensitive {
			masked := make([]string, len(v))
			for i := range v {
				masked[i] = maskValue
			}
			out[k] = masked
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}
