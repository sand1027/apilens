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
	// GraphQL asserts over the GraphQL-over-HTTP envelope (data / errors)
	// rather than HTTP status alone — GraphQL often returns 200 with
	// errors in the body.
	GraphQL *GraphQLAssertSpec
	// DB is a v9 addition (plan.md v9: "Database assertions (opt-in
	// plugin)") — keyed by the connection name configured under
	// db.connections in config.yaml.
	DB map[string]DBSpec
}

// GraphQLAssertSpec is the assert.graphql YAML block.
type GraphQLAssertSpec struct {
	// NoErrors true: fail if errors is a non-empty array.
	// NoErrors false: fail if there are no errors (assert a GraphQL error).
	NoErrors *bool
	// HasData true: data must be present and non-null.
	// HasData false: data must be missing or null.
	HasData *bool
	// ErrorContains fails unless some error message contains this substring.
	ErrorContains *string
}

// DBSpec is one db.<connection> assertion block (plan.md v9, extended for
// MongoDB support). Exactly one of RowCountEquals / Exists / Equals should
// be set per query — testdef does not enforce mutual exclusion here since
// a single query result can reasonably be checked more than one way (e.g.
// both "exists" and an "equals" on a specific column), but at least one of
// Query or Collection must always be set (testdef.compileAssert enforces
// that these two are also mutually exclusive with each other — a block is
// either SQL-shaped or MongoDB-shaped, never both).
type DBSpec struct {
	// Query/Args are the SQL shape: a read-only SELECT (validated at
	// Compile time by dbassert.ValidateReadOnly) plus its positional
	// placeholder arguments.
	Query string
	Args  []any
	// Collection/Filter are the MongoDB shape: a collection name plus a
	// filter document (decoded from YAML as a plain map[string]any/
	// []any/scalar tree, the same shape driver methods like
	// Collection.Find already accept directly as a BSON-convertible
	// filter).
	Collection string
	Filter     any
	// Field names which document field Equals compares against
	// (MongoDB only — the SQL shape instead compares the first column
	// of Query's result row). Defaults to "_id" when Equals is set but
	// Field is left blank.
	Field string
	// RowCountEquals asserts the query/filter returns exactly this many
	// rows/documents.
	RowCountEquals *int
	// Exists asserts the query/filter returns at least one row/document
	// (true) or none (false).
	Exists *bool
	// Equals asserts the first column of the first row (SQL) or the
	// named Field of the first matched document (MongoDB), formatted as
	// a string, equals this value's string form.
	Equals any
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
	KindDBRowCount         AssertionKind = "db.row_count_equals"
	KindDBExists           AssertionKind = "db.exists"
	KindDBEquals           AssertionKind = "db.equals"
	KindGraphQLNoErrors    AssertionKind = "graphql.no_errors"
	KindGraphQLHasData     AssertionKind = "graphql.has_data"
	KindGraphQLErrorContains AssertionKind = "graphql.error_contains"
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
