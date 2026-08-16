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
	// GraphQL, when set, is serialized to a GraphQL-over-HTTP JSON body
	// (query / variables / operationName) at interpolate time. Mutually
	// exclusive with Body.
	GraphQL *GraphQLTemplate
}

// GraphQLTemplate is the request.graphql YAML block.
type GraphQLTemplate struct {
	Query         string
	Variables     any
	OperationName string
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
	// Cleanup runs after this test's own request completes — regardless
	// of whether the test passed, failed, or errored — to delete
	// whatever database record(s) it created. See CleanupSpec's doc
	// comment for why this exists as its own top-level block rather than
	// living under Assert.
	Cleanup     CleanupSpec
	Timeout     time.Duration // effective per-test timeout after defaults
	Retries     int

	// Version is the DSL version this test was written against (1 or 2).
	// Files with no explicit "version:" field are treated as 1
	// (docs/06-test-dsl.md section 1). Version 2 is required to use
	// response chaining ({{responses.<id>...}}) — plan.md v9 / DSL v2.
	Version int
	// ID names this test so a LATER test in the same suite can reference
	// its response via {{responses.<ID>...}}. Optional — most tests never
	// need one. Must be unique within a suite when set (enforced by
	// app.RunSuite, since uniqueness is a suite-wide property, not a
	// per-file one).
	ID string
	// UsesChaining is true if this test's own request template contains a
	// {{responses....}} reference. testrunner/app use this to force
	// sequential execution for suites that need it — chaining a response
	// from test A into test B requires A to have already run
	// (docs/11-risks-and-gaps.md R6: chaining is incompatible with
	// unordered parallel execution).
	UsesChaining bool
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
