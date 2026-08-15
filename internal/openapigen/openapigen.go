// Package openapigen implements plan.md v9's "Automatic test generation
// from OpenAPI examples (now that v7 contracts exist)": read a stored
// OpenAPI 3 document (the same one internal/contract validates against)
// and, for every operation that declares an example, produce a YAML v1
// test case exercising it.
//
// Generation is example-driven, not schema-driven: a request/response
// example is real, author-supplied sample data, so building a test from
// it is filling in what the spec's author already promised, not
// synthesizing new claims from nothing (the same "never invent" line
// internal/specexport draws when it refuses to guess a response schema
// with no captured example to infer from). An operation with no example
// anywhere is skipped, not padded with a placeholder value.
package openapigen

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/domain"
)

// Generated is one produced test, plus which operation it came from (for
// diagnostics/reporting — mirrors internal/generate.Generated's shape).
type Generated struct {
	OperationID string // op.OperationID, or "<METHOD> <path>" if unset
	Test        domain.TestCase
}

// FromDoc walks every path/operation in doc and returns one Generated per
// operation that has a usable request example (or needs none — a bodyless
// GET/DELETE with no parameters still generates, since there's nothing to
// fill in). Operations are visited in the same deterministic
// (path, then method) order internal/specexport and internal/contract
// already use, so repeated runs against an unchanged spec produce
// byte-identical output.
func FromDoc(doc *openapi3.T) []Generated {
	if doc == nil || doc.Paths == nil {
		return nil
	}

	var out []Generated
	for _, rawPath := range sortedPathKeys(doc.Paths) {
		item := doc.Paths.Find(rawPath)
		if item == nil {
			continue
		}
		for _, m := range methodOrder {
			op := m.get(item)
			if op == nil {
				continue
			}
			tc, ok := buildTestCase(m.name, rawPath, op)
			if !ok {
				continue
			}
			id := op.OperationID
			if id == "" {
				id = m.name + " " + rawPath
			}
			out = append(out, Generated{OperationID: id, Test: tc})
		}
	}
	return out
}

func sortedPathKeys(paths *openapi3.Paths) []string {
	keys := make([]string, 0, paths.Len())
	for _, k := range paths.InMatchingOrder() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

var methodOrder = []struct {
	name string
	get  func(*openapi3.PathItem) *openapi3.Operation
}{
	{"GET", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Get }},
	{"POST", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Post }},
	{"PUT", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Put }},
	{"PATCH", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Patch }},
	{"DELETE", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Delete }},
}

// buildTestCase builds one test for method+path+op, returning ok=false
// when the operation has a request body but no usable example for it —
// generating a request with a required body silently omitted would
// produce a test that fails for a reason unrelated to what it's meant to
// check (a validation 400, not the behavior the author cares about).
func buildTestCase(method, rawPath string, op *openapi3.Operation) (domain.TestCase, bool) {
	url, ok := fillPathParams(rawPath, op)
	if !ok {
		return domain.TestCase{}, false
	}

	var body domain.BodyTemplate
	if op.RequestBody != nil && op.RequestBody.Value != nil {
		example, found := requestBodyExample(op.RequestBody.Value)
		if !found {
			// The spec declares a request body but no example exists
			// anywhere to fill it with — skip rather than send an empty
			// body a real API would likely reject for an unrelated
			// reason (docs/11-risks-and-gaps.md's "never invent").
			if op.RequestBody.Value.Required {
				return domain.TestCase{}, false
			}
		} else {
			body.JSON = example
		}
	}

	query := queryParamExamples(op)

	name := op.OperationID
	if name == "" {
		name = method + " " + rawPath
	}

	tc := domain.TestCase{
		Name:        name,
		Version:     1,
		Description: op.Summary,
		Tags:        append([]string{"generated", "openapi-example"}, op.Tags...),
		Request: domain.RequestTemplate{
			Method: domain.NormalizeMethod(method),
			URL:    "{{base_url}}" + url,
			Query:  query,
			Body:   body,
		},
		Assert: buildAssertions(op),
	}
	return tc, true
}

// fillPathParams substitutes every "{name}" path template segment with
// that parameter's declared example (from the parameter itself, or its
// schema's example as a fallback). Returns ok=false if any required path
// parameter has no example anywhere — an unresolved "{id}" left in the
// URL would silently 404 against a real server rather than testing
// anything meaningful.
func fillPathParams(rawPath string, op *openapi3.Operation) (string, bool) {
	result := rawPath
	for _, pr := range op.Parameters {
		if pr.Value == nil || pr.Value.In != "path" {
			continue
		}
		val, found := parameterExample(pr.Value)
		if !found {
			return "", false
		}
		result = strings.ReplaceAll(result, "{"+pr.Value.Name+"}", stringifyExample(val))
	}
	// Any remaining unresolved "{...}" segment means a path parameter
	// wasn't declared with an example — bail rather than send a literal
	// "{id}" as a URL segment.
	if strings.ContainsAny(result, "{}") {
		return "", false
	}
	return result, true
}

// queryParamExamples fills query.* string values from each query
// parameter's example, when one exists. Query parameters without an
// example are simply omitted (unlike path parameters, an absent optional
// query param is a perfectly normal, valid request).
func queryParamExamples(op *openapi3.Operation) map[string]string {
	var query map[string]string
	for _, pr := range op.Parameters {
		if pr.Value == nil || pr.Value.In != "query" {
			continue
		}
		val, found := parameterExample(pr.Value)
		if !found {
			continue
		}
		if query == nil {
			query = map[string]string{}
		}
		query[pr.Value.Name] = stringifyExample(val)
	}
	return query
}

// parameterExample checks the parameter's own Example/Examples first,
// then its schema's Example, in that priority order (docs/07-discovery.md's
// existing convention of preferring the most specific, closest-to-usage
// value first).
func parameterExample(p *openapi3.Parameter) (any, bool) {
	if p.Example != nil {
		return p.Example, true
	}
	if len(p.Examples) > 0 {
		if v, ok := firstExampleValue(p.Examples); ok {
			return v, true
		}
	}
	if p.Schema != nil && p.Schema.Value != nil && p.Schema.Value.Example != nil {
		return p.Schema.Value.Example, true
	}
	return nil, false
}

// requestBodyExample checks the application/json media type's Example,
// then Examples, then its schema's Example.
func requestBodyExample(rb *openapi3.RequestBody) (any, bool) {
	mt := rb.Content.Get("application/json")
	if mt == nil {
		return nil, false
	}
	if mt.Example != nil {
		return mt.Example, true
	}
	if len(mt.Examples) > 0 {
		if v, ok := firstExampleValue(mt.Examples); ok {
			return v, true
		}
	}
	if mt.Schema != nil && mt.Schema.Value != nil && mt.Schema.Value.Example != nil {
		return mt.Schema.Value.Example, true
	}
	return nil, false
}

// firstExampleValue picks the alphabetically-first named example's
// Value, for determinism across repeated generation runs (map iteration
// order is not guaranteed).
func firstExampleValue(examples openapi3.Examples) (any, bool) {
	if len(examples) == 0 {
		return nil, false
	}
	names := make([]string, 0, len(examples))
	for name := range examples {
		names = append(names, name)
	}
	sort.Strings(names)
	ref := examples[names[0]]
	if ref == nil || ref.Value == nil || ref.Value.Value == nil {
		return nil, false
	}
	return ref.Value.Value, true
}

// buildAssertions asserts on the status code the RESPONSE example itself
// declares (the lowest documented 2xx/3xx code, since that's what
// exercising the example should actually produce), falling back to a
// plain "200 expected" only when the spec declares no response codes at
// all (which itself would be an unusual, minimal spec).
func buildAssertions(op *openapi3.Operation) domain.AssertionSpec {
	status := 200
	if op.Responses != nil {
		if code, ok := lowestSuccessCode(op.Responses); ok {
			status = code
		}
	}
	return domain.AssertionSpec{Status: &domain.StatusSpec{Equals: &status}}
}

func lowestSuccessCode(responses *openapi3.Responses) (int, bool) {
	best := 0
	for _, code := range responses.Keys() {
		n, ok := parsePositiveInt(code)
		if !ok || n < 200 || n >= 400 {
			continue
		}
		if best == 0 || n < best {
			best = n
		}
	}
	return best, best != 0
}

func parsePositiveInt(s string) (int, bool) {
	n := 0
	if s == "" {
		return 0, false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int(c-'0')
	}
	return n, true
}

func stringifyExample(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case nil:
		return ""
	default:
		encoded, err := json.Marshal(t)
		if err != nil {
			return ""
		}
		// Strip surrounding quotes if the marshaled form is itself a
		// quoted string (shouldn't normally hit this branch since
		// strings are handled above, but stringify defensively).
		if len(encoded) >= 2 && encoded[0] == '"' && encoded[len(encoded)-1] == '"' {
			var unquoted string
			if err := json.Unmarshal(encoded, &unquoted); err == nil {
				return unquoted
			}
		}
		return string(encoded)
	}
}
