// Package testdef parses the YAML test DSL (docs/06-test-dsl.md) into
// domain.TestCase. One YAML document == one test (ADR-016). Two DSL
// versions are supported: version 1 (the original MVP shape) and version
// 2, which adds response chaining ({{responses.<id>...}} placeholders,
// plan.md v9) — chaining syntax is a compile error under version 1, per
// docs/06-test-dsl.md section 11's explicit rule that chaining must be "a
// later DSL version, not a silent v1 add-on".
package testdef

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// document mirrors the raw YAML shape. yaml.v3 decodes into `any` for the
// most flexible fields (json body, assertion values) so we can validate
// shape ourselves and produce ErrConfig with a clear message rather than a
// generic unmarshal error.
type document struct {
	Version     int      `yaml:"version"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Tags        []string `yaml:"tags"`
	Skip        bool     `yaml:"skip"`
	Retries     *int     `yaml:"retries"`
	// ID names this test so a later test in the same suite can chain off
	// its response (DSL v2 / plan.md v9: "Response chaining in DSL v2").
	// Only meaningful (and only permitted) under version: 2.
	ID string `yaml:"id"`

	Request requestDoc  `yaml:"request"`
	Assert  assertDoc   `yaml:"assert"`
	// Cleanup is a sibling of assert, not nested inside it — see
	// domain.CleanupSpec's doc comment for why teardown is a distinct
	// top-level concept from assertion checks.
	Cleanup cleanupDoc `yaml:"cleanup"`
}

type requestDoc struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Query   map[string]string `yaml:"query"`
	Body    bodyDoc           `yaml:"body"`
	Auth    *authDoc          `yaml:"auth"`
	Timeout string            `yaml:"timeout"`
	GraphQL *graphqlDoc       `yaml:"graphql"`
}

type graphqlDoc struct {
	Query         string `yaml:"query"`
	Variables     any    `yaml:"variables"`
	OperationName string `yaml:"operation"`
}

type bodyDoc struct {
	JSON        any    `yaml:"json"`
	Raw         string `yaml:"raw"`
	ContentType string `yaml:"content_type"`
}

type authDoc struct {
	Type     string `yaml:"type"`
	Token    string `yaml:"token"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Header   string `yaml:"header"`
	Value    string `yaml:"value"`
	Name     string `yaml:"name"`
}

type assertDoc struct {
	Status   *statusDoc           `yaml:"status"`
	Headers  map[string]headerDoc `yaml:"headers"`
	Body     *bodyAssertDoc       `yaml:"body"`
	JSON     map[string]jsonDoc   `yaml:"json"`
	Duration *durationDoc         `yaml:"duration"`
	GraphQL  *graphqlAssertDoc    `yaml:"graphql"`
	// DB is a v9 addition (plan.md v9: "Database assertions"). Key is
	// the connection name configured under db.connections in
	// config.yaml. Each connection's value may be written as either a
	// single mapping (one check) or a list of mappings (multiple checks
	// against the same connection) — dbDocList.UnmarshalYAML normalizes
	// both forms to a slice so compile.go only ever deals with one shape.
	DB map[string]dbDocList `yaml:"db"`
}

type graphqlAssertDoc struct {
	NoErrors      *bool   `yaml:"no_errors"`
	HasData       *bool   `yaml:"has_data"`
	ErrorContains *string `yaml:"error_contains"`
}

type dbDoc struct {
	// Query/Args are the SQL (sqlite/postgres) shape.
	Query string `yaml:"query"`
	Args  []any  `yaml:"args"`
	// Collection/Filter/Field are the MongoDB shape — mutually
	// exclusive with Query/Args (testdef.compileAssert enforces this).
	Collection string `yaml:"collection"`
	Filter     any    `yaml:"filter"`
	Field      string `yaml:"field"`
	// Shared expectations, usable with either shape above.
	RowCountEquals *int  `yaml:"row_count_equals"`
	Exists         *bool `yaml:"exists"`
	Equals         any   `yaml:"equals"`
}

// dbDocList accepts either a single db.<connection> mapping (the original
// v9 shape: one check per connection) or a YAML sequence of mappings (the
// extended shape: multiple checks against the same connection, e.g. one
// per collection a mutation is expected to have written to). Both forms
// decode to the same []dbDoc so compile.go never needs to branch on which
// one was written.
type dbDocList []dbDoc

func (l *dbDocList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var docs []dbDoc
		if err := value.Decode(&docs); err != nil {
			return err
		}
		*l = docs
		return nil
	case yaml.MappingNode:
		var doc dbDoc
		if err := value.Decode(&doc); err != nil {
			return err
		}
		*l = []dbDoc{doc}
		return nil
	default:
		return fmt.Errorf("db.<connection> must be a mapping (one check) or a list of mappings (multiple checks), got %v", value.Kind)
	}
}

type statusDoc struct {
	Equals    *int `yaml:"equals"`
	NotEquals *int `yaml:"not_equals"`
}

type headerDoc struct {
	Exists   *bool   `yaml:"exists"`
	Equals   *string `yaml:"equals"`
	Contains *string `yaml:"contains"`
}

type bodyAssertDoc struct {
	Contains    *string `yaml:"contains"`
	NotContains *string `yaml:"not_contains"`
}

type jsonDoc struct {
	Exists   *bool `yaml:"exists"`
	Equals   any   `yaml:"equals"`
	Contains any   `yaml:"contains"`
	// Schema/Matches/Length are v7 additions (docs/06-test-dsl.md
	// section 11 lists these as explicitly out of v1; plan.md v7 ships
	// them). Schema is an inline JSON Schema document; SchemaFile
	// (v7 addition) loads it from a file relative to the test's own
	// directory so schemas can be shared across tests.
	Schema     any     `yaml:"schema"`
	SchemaFile string  `yaml:"schema_file"`
	Matches    *string `yaml:"matches"`
	Length     *int    `yaml:"length"`
}

type durationDoc struct {
	LessThan *int `yaml:"less_than"`
}

// cleanupDoc mirrors the (MongoDB-only, for now) db.<connection> shape
// used by assertDoc's DB field, but for deletion rather than a read
// check — see domain.CleanupSpec's doc comment. Reuses dbDocList's
// single-mapping-or-list UnmarshalYAML so `cleanup.main` can be written
// as one collection or a list of collections, same convention as
// `assert.db.main`.
type cleanupDoc struct {
	DB map[string]cleanupDocList `yaml:"db"`
}

type cleanupItemDoc struct {
	Collection string `yaml:"collection"`
	Filter     any    `yaml:"filter"`
}

type cleanupDocList []cleanupItemDoc

func (l *cleanupDocList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.SequenceNode:
		var docs []cleanupItemDoc
		if err := value.Decode(&docs); err != nil {
			return err
		}
		*l = docs
		return nil
	case yaml.MappingNode:
		var doc cleanupItemDoc
		if err := value.Decode(&doc); err != nil {
			return err
		}
		*l = []cleanupItemDoc{doc}
		return nil
	default:
		return fmt.Errorf("cleanup.db.<connection> must be a mapping (one target) or a list of mappings, got %v", value.Kind)
	}
}
