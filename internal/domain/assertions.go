package domain

// AssertionSpec is the raw, validated (but not compiled) assertion block
// parsed from YAML. testdef validates shape; assertions.Engine compiles it
// into an AssertionSet of executable checks.
type AssertionSpec struct {
	Status   *StatusSpec
	Headers  map[string]HeaderSpec // key = header name as written in YAML
	Body     *BodySpec
	JSON     map[string]JSONSpec // key = dotted path
	Duration *DurationSpec
}

type StatusSpec struct {
	Equals    *int
	NotEquals *int
}

type HeaderSpec struct {
	Exists   *bool
	Equals   *string
	Contains *string
}

type BodySpec struct {
	Contains    *string
	NotContains *string
}

type JSONSpec struct {
	Exists   *bool
	Equals   any
	Contains any
	// Schema, Matches, and Length are v7 additions (plan.md v7:
	// "JSON Schema assertions ... Richer JSON path / regex assertions ...
	// Array length assertions" — explicitly out of scope for v1 per
	// docs/06-test-dsl.md section 11, now shipping).
	Schema  any     // inline JSON Schema document (map[string]any) validated against the value at Path
	Matches *string // regex; value at Path must be a string matching this pattern
	Length  *int    // value at Path must be a string, array, or object with this length
}

type DurationSpec struct {
	LessThan *int // milliseconds
}

// AssertionKind names a single compiled check, e.g. "status.equals" or
// "json.exists". Used in reports and for the plugin Evaluator.Kind() match.
type AssertionKind string

const (
	KindStatusEquals    AssertionKind = "status.equals"
	KindStatusNotEquals AssertionKind = "status.not_equals"
	KindHeaderExists    AssertionKind = "header.exists"
	KindHeaderEquals    AssertionKind = "header.equals"
	KindHeaderContains  AssertionKind = "header.contains"
	KindBodyContains    AssertionKind = "body.contains"
	KindBodyNotContains AssertionKind = "body.not_contains"
	KindJSONExists      AssertionKind = "json.exists"
	KindJSONEquals      AssertionKind = "json.equals"
	KindJSONContains    AssertionKind = "json.contains"
	KindJSONSchema      AssertionKind = "json.schema"
	KindJSONMatches     AssertionKind = "json.matches"
	KindJSONLength      AssertionKind = "json.length"
	KindDurationLess    AssertionKind = "duration.less_than"
)

// AssertionResult is the outcome of evaluating a single compiled check
// against an Exchange.
type AssertionResult struct {
	Kind     AssertionKind
	Target   string // header name / json path, empty for status/duration
	Passed   bool
	Expected string
	Actual   string
	Reason   string // populated on failure or evaluation error, e.g. "response is not JSON"
}

// AssertionSet is the compiled, executable form of an AssertionSpec. The
// concrete Check values live in internal/assertions; domain only needs the
// count for reporting before execution.
type AssertionSet struct {
	Checks []Check
}

// Check is implemented by internal/assertions.compiledCheck. Declared here
// so domain.AssertionSet can hold it without importing internal/assertions
// (which would create an import cycle, since assertions imports domain).
type Check interface {
	Eval(ex Exchange) AssertionResult
}
