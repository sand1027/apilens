package contract

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/domain"
)

func TestLoad_ValidSpecSucceeds(t *testing.T) {
	doc, err := Load("../../testdata/openapi/valid/contract-spec.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if doc.Paths.Len() != 2 {
		t.Errorf("expected 2 paths, got %d", doc.Paths.Len())
	}
}

func TestLoad_MissingFileFails(t *testing.T) {
	_, err := Load("../../testdata/openapi/valid/does-not-exist.yaml")
	if err == nil {
		t.Fatal("expected error loading a missing file")
	}
	if !errors.Is(err, domain.ErrConfig) {
		t.Errorf("expected ErrConfig, got %v", err)
	}
}

func TestLoad_BrokenSpecFails(t *testing.T) {
	_, err := Load("../../testdata/openapi/invalid/broken.yaml")
	if err == nil {
		t.Fatal("expected error loading a structurally broken spec")
	}
	if !errors.Is(err, domain.ErrConfig) {
		t.Errorf("expected ErrConfig, got %v", err)
	}
}

func mustLoadFixtureSpec(t *testing.T) *openapi3.T {
	t.Helper()
	doc, err := Load("../../testdata/openapi/valid/contract-spec.yaml")
	if err != nil {
		t.Fatalf("Load fixture spec: %v", err)
	}
	return doc
}

func TestRun_MatchingCapturePasses(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	ex := domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 200, Headers: http.Header{},
			Body: []byte(`{"data":{"id":1,"name":"Ada","email":"ada@example.com"}}`),
		},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/{id}"): ex,
	}
	report, err := Run(context.Background(), doc, nil, Options{Examples: examples})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Passed != 1 {
		t.Errorf("expected 1 passed, got counts=%+v results=%+v", report.Counts, report.Results)
	}
}

func TestRun_BreakingCaptureFails(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	// missing required "email" field — a breaking response-shape change.
	ex := domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 200, Headers: http.Header{},
			Body: []byte(`{"data":{"id":1,"name":"Ada"}}`),
		},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/{id}"): ex,
	}
	report, err := Run(context.Background(), doc, nil, Options{Examples: examples})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Failed != 1 {
		t.Fatalf("expected 1 failed, got counts=%+v", report.Counts)
	}
	result := findResult(t, report, "/api/users/{id}")
	if len(result.Assertions) != 1 || result.Assertions[0].Reason == "" {
		t.Errorf("expected a non-empty failure reason, got %+v", result)
	}
}

// findResult locates the TestResult for urlPath, since /api/users and
// /api/users/{id} both appear in the fixture spec and their relative
// order in report.Results is an implementation detail, not something
// tests should assume.
func findResult(t *testing.T, report domain.Report, urlPath string) domain.TestResult {
	t.Helper()
	for _, r := range report.Results {
		if r.URL == urlPath {
			return r
		}
	}
	t.Fatalf("no result for %s in %+v", urlPath, report.Results)
	return domain.TestResult{}
}

func TestRun_NoExampleNoProbeSkips(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	report, err := Run(context.Background(), doc, nil, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Skipped != report.Counts.Tests {
		t.Errorf("expected every operation to skip without an example or probe, got %+v", report.Counts)
	}
	if report.Counts.Failed != 0 {
		t.Errorf("expected no false failures when there's nothing to check, got %+v", report.Counts)
	}
}

func TestRun_FallsBackToProbeWhenNoExample(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	probeCalled := false
	probe := func(ctx context.Context, method, path string) (domain.Exchange, error) {
		probeCalled = true
		return domain.Exchange{
			Response: domain.HTTPResponse{
				StatusCode: 200, Headers: http.Header{},
				Body: []byte(`{"data":[{"id":1,"name":"Ada","email":"ada@example.com"}]}`),
			},
		}, nil
	}
	report, err := Run(context.Background(), doc, probe, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !probeCalled {
		t.Error("expected probe to be called when no capture is available")
	}
	if report.Counts.Passed == 0 {
		t.Errorf("expected at least one passing probe-backed check, got %+v", report.Counts)
	}
}

func TestRun_ProbeErrorErrorsThatResult(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	probe := func(ctx context.Context, method, path string) (domain.Exchange, error) {
		return domain.Exchange{}, errors.New("connection refused")
	}
	report, err := Run(context.Background(), doc, probe, Options{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Errored != report.Counts.Tests {
		t.Errorf("expected every operation to error when the probe fails, got %+v", report.Counts)
	}
}

func TestRun_NoPathsIsConfigError(t *testing.T) {
	_, err := Run(context.Background(), nil, nil, Options{})
	if !errors.Is(err, domain.ErrConfig) {
		t.Errorf("expected ErrConfig for a nil doc, got %v", err)
	}
}

func TestRun_ExampleTakesPriorityOverProbe(t *testing.T) {
	doc := mustLoadFixtureSpec(t)
	var probedPaths []string
	probe := func(ctx context.Context, method, path string) (domain.Exchange, error) {
		probedPaths = append(probedPaths, path)
		return domain.Exchange{
			Response: domain.HTTPResponse{
				StatusCode: 200, Headers: http.Header{},
				Body: []byte(`{"data":[{"id":1,"name":"Ada","email":"ada@example.com"}]}`),
			},
		}, nil
	}
	ex := domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 200, Headers: http.Header{},
			Body: []byte(`{"data":{"id":1,"name":"Ada","email":"ada@example.com"}}`),
		},
	}
	// Supply an example only for /api/users/{id}; /api/users has none, so
	// it must fall through to the probe — this test asserts the OTHER
	// endpoint's probe call never happens for the one WITH an example.
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/{id}"): ex,
	}
	if _, err := Run(context.Background(), doc, probe, Options{Examples: examples}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, p := range probedPaths {
		if p == "/api/users/{id}" {
			t.Error("expected the probe NOT to be called for an endpoint with a matching example")
		}
	}
}
