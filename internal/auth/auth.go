// Package auth implements the auth scheme plugin port from
// docs/03-plugins.md section 7 / docs/04-interfaces.md section 7. Schemes
// only attach credentials to a request; they never log them — display
// masking is always internal/security.Redactor's job.
package auth

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

// Credentials holds the already-interpolated (no "{{var}}" left) fields
// needed by any built-in scheme. Only the fields relevant to Kind are set.
type Credentials struct {
	Token    string
	Username string
	Password string
	Header   string
	Value    string
	Name     string
}

// Scheme is the auth plugin contract.
type Scheme interface {
	Kind() string
	Apply(req *domain.HTTPRequest, creds Credentials) error
}

// Registry looks up a Scheme by kind. Built-ins are registered by
// RegisterBuiltins; internal/plugins.Host wraps a Registry for the wider
// plugin surface.
type Registry struct {
	schemes map[string]Scheme
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{schemes: make(map[string]Scheme)}
}

// Register adds a scheme, keyed by its Kind() (case-insensitive).
func (r *Registry) Register(s Scheme) {
	r.schemes[strings.ToLower(s.Kind())] = s
}

// Get looks up a scheme by kind.
func (r *Registry) Get(kind string) (Scheme, bool) {
	s, ok := r.schemes[strings.ToLower(kind)]
	return s, ok
}

// RegisterBuiltins registers bearer, basic, apikey, and cookie — the MVP
// built-in kinds from docs/03-plugins.md section 8.
func RegisterBuiltins(r *Registry) {
	r.Register(bearerScheme{})
	r.Register(basicScheme{})
	r.Register(apiKeyScheme{})
	r.Register(cookieScheme{})
}

// NewDefaultRegistry builds a Registry with all built-ins registered.
func NewDefaultRegistry() *Registry {
	r := NewRegistry()
	RegisterBuiltins(r)
	return r
}

type bearerScheme struct{}

func (bearerScheme) Kind() string { return "bearer" }
func (bearerScheme) Apply(req *domain.HTTPRequest, creds Credentials) error {
	ensureHeaders(req)
	req.Headers.Set("Authorization", "Bearer "+creds.Token)
	return nil
}

type basicScheme struct{}

func (basicScheme) Kind() string { return "basic" }
func (basicScheme) Apply(req *domain.HTTPRequest, creds Credentials) error {
	ensureHeaders(req)
	dummy := &http.Request{Header: http.Header{}}
	dummy.SetBasicAuth(creds.Username, creds.Password)
	req.Headers.Set("Authorization", dummy.Header.Get("Authorization"))
	return nil
}

type apiKeyScheme struct{}

func (apiKeyScheme) Kind() string { return "apikey" }
func (apiKeyScheme) Apply(req *domain.HTTPRequest, creds Credentials) error {
	ensureHeaders(req)
	name := creds.Header
	if name == "" {
		name = "X-API-Key"
	}
	req.Headers.Set(name, creds.Value)
	return nil
}

type cookieScheme struct{}

func (cookieScheme) Kind() string { return "cookie" }
func (cookieScheme) Apply(req *domain.HTTPRequest, creds Credentials) error {
	ensureHeaders(req)
	name := creds.Name
	if name == "" {
		name = "session"
	}
	pair := name + "=" + creds.Value
	if existing := req.Headers.Get("Cookie"); existing != "" {
		req.Headers.Set("Cookie", existing+"; "+pair)
	} else {
		req.Headers.Set("Cookie", pair)
	}
	return nil
}

func ensureHeaders(req *domain.HTTPRequest) {
	if req.Headers == nil {
		req.Headers = http.Header{}
	}
}

// ErrUnknownKind is returned when a test references an auth type with no
// registered scheme.
func ErrUnknownKind(kind string) error {
	return domain.NewConfigError(fmt.Sprintf("unknown auth type %q", kind), nil)
}
