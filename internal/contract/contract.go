// Package contract implements v7's "Contract testing against a stored
// spec" (plan.md v7): load a previously written OpenAPI document (e.g.
// one produced by internal/specexport, or a hand-written spec) and check
// that live or captured responses still match the response schemas it
// declares. Results reuse domain.TestResult/AssertionResult so contract
// runs can be printed by the exact same reporters as a normal
// `apilens run` (docs/01-architecture.md: reporters only format, they
// never recompute pass/fail — ADR-022 applies equally here).
package contract

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/domain"
)

// Load parses and validates an OpenAPI 3 / Swagger 2 document at path. A
// malformed or structurally invalid spec is a config error (exit 2),
// caught before any probing happens — same discipline as
// internal/discovery/providers/openapi's parseFile for explicit --path
// input (ADR-008: broken spec provided explicitly is propagated, not
// silently skipped).
func Load(path string) (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromFile(path)
	if err != nil {
		return nil, domain.NewConfigError("loading OpenAPI spec "+path, err)
	}
	if doc.OpenAPI == "" {
		return nil, domain.NewConfigError(path+" is missing the openapi version field", nil)
	}
	if err := doc.Validate(loader.Context); err != nil {
		return nil, domain.NewConfigError("invalid OpenAPI spec "+path, err)
	}
	return doc, nil
}

// Prober performs a live HTTP call for one contract check and returns the
// resulting Exchange. Supplied by the caller (internal/app) so this
// package stays free of HTTP/environment policy — the same layering
// Inspect uses for its --live probe (internal/app/discover.go).
type Prober func(ctx context.Context, method, path string) (domain.Exchange, error)

// Options configures Run.
type Options struct {
	// Examples supplies a pre-captured exchange for an endpoint, keyed by
	// domain.NewEndpointID(method, path) — the same key shape
	// internal/specexport uses. When an endpoint has a matching entry
	// here, Run validates it directly and never calls Probe for that
	// endpoint (docs pattern: captures can stand in for a live probe).
	Examples map[domain.EndpointID]domain.Exchange
}

// Run walks every operation declared in doc, resolves an Exchange for it
// (from opts.Examples first, falling back to probe), and validates the
// response body against whichever response schema the spec declares for
// the status code actually returned. An operation with no matching
// response schema for the returned status — or no way to obtain an
// Exchange at all — is reported as StatusSkipped, not a failure: contract
// testing can only judge what the spec actually promises
// (docs/11-risks-and-gaps.md's general "do not invent findings" stance).
func Run(ctx context.Context, doc *openapi3.T, probe Prober, opts Options) (domain.Report, error) {
	if doc == nil || doc.Paths == nil || doc.Paths.Len() == 0 {
		return domain.Report{}, domain.NewConfigError("OpenAPI spec has no paths to contract test", nil)
	}

	var results []domain.TestResult
	for _, p := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Find(p)
		if item == nil {
			continue
		}
		for _, m := range methodOrder {
			op := m.get(item)
			if op == nil {
				continue
			}
			results = append(results, runOne(ctx, m.name, p, op, probe, opts))
		}
	}
	if len(results) == 0 {
		return domain.Report{}, domain.NewConfigError("OpenAPI spec declared no operations to contract test", nil)
	}

	return buildReport(results), nil
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
	{"HEAD", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Head }},
	{"OPTIONS", func(pi *openapi3.PathItem) *openapi3.Operation { return pi.Options }},
}

func runOne(ctx context.Context, method, path string, op *openapi3.Operation, probe Prober, opts Options) domain.TestResult {
	result := domain.TestResult{
		Name:   method + " " + path,
		Method: domain.Method(method),
		URL:    path,
	}

	ex, ok, err := resolveExchange(ctx, method, path, probe, opts)
	if err != nil {
		result.Status = domain.StatusErrored
		result.Error = err.Error()
		return result
	}
	if !ok {
		result.Status = domain.StatusSkipped
		result.Error = "no captured exchange and no live probe available for this endpoint"
		return result
	}

	result.HTTPStatus = ex.Response.StatusCode
	result.DurationMS = ex.Timing.Duration.Milliseconds()

	schema, found := responseSchemaFor(op, ex.Response.StatusCode)
	if !found {
		result.Status = domain.StatusSkipped
		result.Error = fmt.Sprintf("spec declares no JSON response schema for status %d on %s", ex.Response.StatusCode, result.Name)
		return result
	}

	ar := validateAgainstSchema(schema, ex.Response.Body)
	result.Assertions = []domain.AssertionResult{ar}
	if ar.Passed {
		result.Status = domain.StatusPassed
	} else {
		result.Status = domain.StatusFailed
	}
	return result
}

// resolveExchange prefers a supplied capture over probing — a capture is
// cheaper and deterministic, and it's the only option for non-GET
// operations or paths with unresolved template parameters.
func resolveExchange(ctx context.Context, method, path string, probe Prober, opts Options) (domain.Exchange, bool, error) {
	if opts.Examples != nil {
		if ex, ok := opts.Examples[domain.NewEndpointID(domain.Method(method), path)]; ok {
			return ex, true, nil
		}
	}
	if probe == nil {
		return domain.Exchange{}, false, nil
	}
	ex, err := probe(ctx, method, path)
	if err != nil {
		return domain.Exchange{}, false, err
	}
	return ex, true, nil
}

// responseSchemaFor finds the JSON schema the spec declares for
// statusCode, honoring OpenAPI's patterned "2XX" fallback via
// Responses.Status (see openapi3.Responses.Status godoc).
func responseSchemaFor(op *openapi3.Operation, statusCode int) (*openapi3.Schema, bool) {
	if op.Responses == nil {
		return nil, false
	}
	ref := op.Responses.Status(statusCode)
	if ref == nil || ref.Value == nil {
		return nil, false
	}
	mt := ref.Value.Content.Get("application/json")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		return nil, false
	}
	return mt.Schema.Value, true
}

// validateAgainstSchema checks body against schema using kin-openapi's own
// Schema.VisitJSON — reusing the same schema engine the spec was parsed
// with, rather than re-encoding to a different JSON Schema dialect
// (ADR-008).
func validateAgainstSchema(schema *openapi3.Schema, body []byte) domain.AssertionResult {
	if len(body) == 0 {
		return domain.AssertionResult{
			Kind: domain.KindJSONSchema, Passed: false,
			Expected: "matches spec schema", Reason: "response body is empty",
		}
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return domain.AssertionResult{
			Kind: domain.KindJSONSchema, Passed: false,
			Expected: "matches spec schema", Reason: "response is not JSON",
		}
	}
	if err := schema.VisitJSON(parsed); err != nil {
		return domain.AssertionResult{
			Kind:     domain.KindJSONSchema,
			Passed:   false,
			Expected: "matches spec schema",
			Actual:   fmt.Sprintf("%v", parsed),
			Reason:   err.Error(),
		}
	}
	return domain.AssertionResult{
		Kind:     domain.KindJSONSchema,
		Passed:   true,
		Expected: "matches spec schema",
		Actual:   "matches spec schema",
	}
}

func buildReport(results []domain.TestResult) domain.Report {
	counts := domain.Counts{Tests: len(results)}
	var totalMS int64
	for _, r := range results {
		totalMS += r.DurationMS
		switch r.Status {
		case domain.StatusPassed:
			counts.Passed++
		case domain.StatusFailed:
			counts.Failed++
		case domain.StatusErrored:
			counts.Errored++
		case domain.StatusSkipped:
			counts.Skipped++
		}
	}
	return domain.Report{Counts: counts, DurationMS: totalMS, Results: results}
}
