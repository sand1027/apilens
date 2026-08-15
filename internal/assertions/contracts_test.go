package assertions

import (
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

// --- json.schema ---

func TestJSONSchema_PassesForMatchingShape(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"id":1,"name":"Ada"}}`)
	schema := map[string]any{
		"type":     "object",
		"required": []any{"id", "name"},
		"properties": map[string]any{
			"id":   map[string]any{"type": "integer"},
			"name": map[string]any{"type": "string"},
		},
	}
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Schema: schema}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.schema to pass, got %+v", results[0])
	}
}

func TestJSONSchema_FailsForMissingRequiredProperty(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"id":1}}`)
	schema := map[string]any{
		"type":     "object",
		"required": []any{"id", "name"},
	}
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Schema: schema}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.schema to fail for missing required property")
	}
	if results[0].Reason == "" {
		t.Error("expected a non-empty failure reason")
	}
}

func TestJSONSchema_FailsForWrongType(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"id":"not-a-number"}}`)
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"id": map[string]any{"type": "integer"},
		},
	}
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Schema: schema}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.schema to fail for a type mismatch")
	}
}

func TestJSONSchema_MalformedSchemaFailsAtCompileTime(t *testing.T) {
	// "type" must be a string or array of strings — this is a malformed
	// JSON Schema document (docs/06-test-dsl.md section 12: bad DSL is
	// caught at Compile time, before any HTTP call).
	badSchema := map[string]any{"type": 12345}
	e := New()
	_, err := e.Compile(domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Schema: badSchema}},
	})
	if err == nil {
		t.Fatal("expected Compile to reject a malformed JSON Schema")
	}
}

func TestJSONSchema_MissingPathFails(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`)
	schema := map[string]any{"type": "object"}
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Schema: schema}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.schema to fail when the path is missing")
	}
}

// --- json.matches ---

func TestJSONMatches_PassesForMatchingString(t *testing.T) {
	ex := exchangeWithJSON(200, `{"email":"ada@example.com"}`)
	pattern := `^[^@]+@[^@]+\.[^@]+$`
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"email": {Matches: &pattern}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.matches to pass, got %+v", results[0])
	}
}

func TestJSONMatches_FailsForNonMatchingString(t *testing.T) {
	ex := exchangeWithJSON(200, `{"email":"not-an-email"}`)
	pattern := `^[^@]+@[^@]+\.[^@]+$`
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"email": {Matches: &pattern}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.matches to fail for a non-matching string")
	}
}

func TestJSONMatches_FailsForNonStringValue(t *testing.T) {
	ex := exchangeWithJSON(200, `{"id":1}`)
	pattern := `\d+`
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"id": {Matches: &pattern}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.matches to fail for a non-string value")
	}
	if results[0].Reason != "value is not a string" {
		t.Errorf("Reason = %q, want %q", results[0].Reason, "value is not a string")
	}
}

func TestJSONMatches_InvalidRegexFailsAtCompileTime(t *testing.T) {
	badPattern := "^[^@]+@("
	e := New()
	_, err := e.Compile(domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"email": {Matches: &badPattern}},
	})
	if err == nil {
		t.Fatal("expected Compile to reject an invalid regular expression")
	}
}

// --- json.length ---

func TestJSONLength_PassesForMatchingArrayLength(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":[1,2,3]}`)
	want := 3
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.length to pass, got %+v", results[0])
	}
}

func TestJSONLength_FailsForMismatchedArrayLength(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":[1,2,3]}`)
	want := 5
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.length to fail for mismatched array length")
	}
	if results[0].Expected != "5" || results[0].Actual != "3" {
		t.Errorf("unexpected expected/actual: %+v", results[0])
	}
}

func TestJSONLength_WorksForStringCharCount(t *testing.T) {
	ex := exchangeWithJSON(200, `{"name":"Ada"}`)
	want := 3
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"name": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.length to pass for string char count, got %+v", results[0])
	}
}

func TestJSONLength_WorksForObjectKeyCount(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"a":1,"b":2}}`)
	want := 2
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.length to pass for object key count, got %+v", results[0])
	}
}

func TestJSONLength_FailsForValueWithNoLength(t *testing.T) {
	ex := exchangeWithJSON(200, `{"count":42}`)
	want := 2
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"count": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.length to fail for a number (no length)")
	}
	if results[0].Reason == "" {
		t.Error("expected a non-empty failure reason")
	}
}

func TestJSONLength_FailsForMissingPath(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`)
	want := 1
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"missing": {Length: &want}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.length to fail when the path is missing")
	}
}
