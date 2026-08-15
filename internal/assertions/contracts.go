// Contract assertion kinds added in v7 (plan.md v7: "JSON Schema
// assertions ... richer JSON path / regex assertions ... array length
// assertions"). These were explicitly out of scope for v1
// (docs/06-test-dsl.md section 11) and are compiled the same way as every
// other assertion kind: bad DSL (a malformed schema, an invalid regex) is
// caught at Compile time — before any HTTP call — per
// docs/06-test-dsl.md section 12.
package assertions

import (
	"fmt"
	"regexp"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// --- json.schema ---

type jsonSchemaCheck struct {
	path   string
	schema *jsonschema.Schema
	// raw is kept only for the Expected column in results; the compiled
	// *jsonschema.Schema has no cheap round-trip back to source text.
	raw any
}

// newJSONSchemaCheck compiles rawSchema (a decoded JSON Schema document,
// e.g. from YAML `assert.json.<path>.schema` or a loaded schema_file) once
// at Compile time. A malformed schema is a config error, never a runtime
// assertion failure.
func newJSONSchemaCheck(path string, rawSchema any) (domain.Check, error) {
	c := jsonschema.NewCompiler()
	// A fixed resource name is fine — each check gets its own Compiler
	// instance, so there's no cross-test collision.
	if err := c.AddResource("schema.json", rawSchema); err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	sch, err := c.Compile("schema.json")
	if err != nil {
		return nil, fmt.Errorf("invalid JSON Schema: %w", err)
	}
	return jsonSchemaCheck{path: path, schema: sch, raw: rawSchema}, nil
}

func (c jsonSchemaCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONSchema, c.path, err)
	}
	if !found {
		return domain.AssertionResult{
			Kind: domain.KindJSONSchema, Target: c.path, Passed: false,
			Reason: missingReason(false, c.path),
		}
	}
	if err := c.schema.Validate(val); err != nil {
		return domain.AssertionResult{
			Kind:     domain.KindJSONSchema,
			Target:   c.path,
			Passed:   false,
			Expected: "matches schema",
			Actual:   fmt.Sprintf("%v", val),
			Reason:   err.Error(),
		}
	}
	return domain.AssertionResult{
		Kind:     domain.KindJSONSchema,
		Target:   c.path,
		Passed:   true,
		Expected: "matches schema",
		Actual:   "matches schema",
	}
}

// --- json.matches (regex) ---

type jsonMatchesCheck struct {
	path    string
	pattern *regexp.Regexp
}

// newJSONMatchesCheck compiles the regex once at Compile time — an invalid
// pattern is a config error, not a per-test-run surprise.
func newJSONMatchesCheck(path, pattern string) (domain.Check, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regular expression %q: %w", pattern, err)
	}
	return jsonMatchesCheck{path: path, pattern: re}, nil
}

func (c jsonMatchesCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONMatches, c.path, err)
	}
	if !found {
		return domain.AssertionResult{
			Kind: domain.KindJSONMatches, Target: c.path, Passed: false,
			Expected: c.pattern.String(), Reason: missingReason(false, c.path),
		}
	}
	s, ok := val.(string)
	if !ok {
		return domain.AssertionResult{
			Kind: domain.KindJSONMatches, Target: c.path, Passed: false,
			Expected: c.pattern.String(),
			Actual:   fmt.Sprintf("%v", val),
			Reason:   "value is not a string",
		}
	}
	passed := c.pattern.MatchString(s)
	return domain.AssertionResult{
		Kind:     domain.KindJSONMatches,
		Target:   c.path,
		Passed:   passed,
		Expected: c.pattern.String(),
		Actual:   s,
	}
}

// --- json.length ---
// Applies to strings (character count), arrays, and objects (key count) —
// plan.md v7's "array length assertions" extended to the other JSON
// container types since the dotted-path lookup already returns any of them.

type jsonLengthCheck struct {
	path string
	want int
}

func (c jsonLengthCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONLength, c.path, err)
	}
	if !found {
		return domain.AssertionResult{
			Kind: domain.KindJSONLength, Target: c.path, Passed: false,
			Expected: fmt.Sprintf("%d", c.want), Reason: missingReason(false, c.path),
		}
	}
	length, ok := jsonLength(val)
	if !ok {
		return domain.AssertionResult{
			Kind: domain.KindJSONLength, Target: c.path, Passed: false,
			Expected: fmt.Sprintf("%d", c.want),
			Reason:   "value has no length (not a string, array, or object)",
		}
	}
	return domain.AssertionResult{
		Kind:     domain.KindJSONLength,
		Target:   c.path,
		Passed:   length == c.want,
		Expected: fmt.Sprintf("%d", c.want),
		Actual:   fmt.Sprintf("%d", length),
	}
}

func jsonLength(val any) (int, bool) {
	switch v := val.(type) {
	case string:
		return len(v), true
	case []any:
		return len(v), true
	case map[string]any:
		return len(v), true
	default:
		return 0, false
	}
}
