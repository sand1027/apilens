package domain

import "time"

// RequestTemplate is the not-yet-interpolated request as parsed from YAML.
// String fields may contain "{{var}}" placeholders that environment.Resolver
// expands into a concrete HTTPRequest before execution.
type RequestTemplate struct {
	Method  Method
	URL     string
	Headers map[string]string
	Query   map[string]string
	Body    BodyTemplate
	Auth    *AuthTemplate
	Timeout time.Duration // zero means "use suite default"
}

// BodyTemplate holds either a JSON body (arbitrary structure, string leaves
// may contain placeholders) or a raw body with an explicit content type.
// Exactly one of JSON / Raw may be set — testdef enforces that at compile
// time (docs/06-test-dsl.md section 3).
type BodyTemplate struct {
	JSON        any
	Raw         string
	ContentType string
}

func (b BodyTemplate) IsEmpty() bool {
	return b.JSON == nil && b.Raw == "" && b.ContentType == ""
}

// AuthTemplate mirrors the `request.auth` block in the YAML DSL
// (docs/06-test-dsl.md section 8).
type AuthTemplate struct {
	Type     string // "bearer" | "basic" | "apikey" | "cookie"
	Token    string
	Username string
	Password string
	Header   string
	Value    string
	Name     string
}

// TestCase is a compiled test, ready for the test runner. Request still
// contains placeholders; Assert is a validated (but not yet compiled into
// executable checks) assertion spec.
type TestCase struct {
	Name        string
	File        string
	Description string
	Tags        []string
	Skip        bool
	Request     RequestTemplate
	Assert      AssertionSpec
	Timeout     time.Duration // effective per-test timeout after defaults
	Retries     int
}

// HasTag reports whether the test case carries the given tag (case-sensitive,
// matches YAML as written).
func (t TestCase) HasTag(tag string) bool {
	for _, x := range t.Tags {
		if x == tag {
			return true
		}
	}
	return false
}
