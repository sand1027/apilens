// Package testdef parses the YAML v1 test DSL (docs/06-test-dsl.md) into
// domain.TestCase. One YAML document == one test (ADR-016).
package testdef

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

	Request requestDoc `yaml:"request"`
	Assert  assertDoc  `yaml:"assert"`
}

type requestDoc struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Query   map[string]string `yaml:"query"`
	Body    bodyDoc           `yaml:"body"`
	Auth    *authDoc          `yaml:"auth"`
	Timeout string            `yaml:"timeout"`
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
