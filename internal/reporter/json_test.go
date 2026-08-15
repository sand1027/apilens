package reporter

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestJSONReporter_SchemaMatchesDocumentedShape(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	rep.Start(domain.SuiteMeta{Env: "local", TotalTests: 1})

	report := domain.Report{
		Env: "local",
		Counts: domain.Counts{
			Tests: 1, Passed: 0, Failed: 1, Errored: 0, Skipped: 0,
		},
		DurationMS: 534,
		Results: []domain.TestResult{
			{
				Name:       "Get User",
				File:       ".apilens/tests/users/get-user.yaml",
				Status:     domain.StatusFailed,
				Method:     "GET",
				URL:        "http://localhost:5000/api/users/1",
				HTTPStatus: 500,
				DurationMS: 218,
				Assertions: []domain.AssertionResult{
					{Kind: domain.KindStatusEquals, Passed: false, Expected: "200", Actual: "500"},
				},
			},
		},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}

	// Check top-level keys match docs/11-risks-and-gaps.md G25.
	for _, key := range []string{"version", "env", "counts", "success_percent", "duration_ms", "results"} {
		if _, ok := decoded[key]; !ok {
			t.Errorf("missing top-level key %q in JSON output: %s", key, buf.String())
		}
	}
	if decoded["version"].(float64) != 1 {
		t.Errorf("version = %v, want 1", decoded["version"])
	}
	results := decoded["results"].([]any)
	if len(results) != 1 {
		t.Fatalf("results length = %d, want 1", len(results))
	}
	first := results[0].(map[string]any)
	if first["status"] != "failed" {
		t.Errorf("results[0].status = %v, want failed", first["status"])
	}
}

func TestJSONReporter_NeverContainsRawAuthorizationValue(t *testing.T) {
	// docs/09-security.md section 3: JSON reports must never include raw
	// Authorization values. The JSON reporter's schema has no header
	// field at all by default, so this is really a structural guarantee —
	// verify no assertion or field in the encoded document exposes a
	// header value we didn't put there ourselves.
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	report := domain.Report{
		Results: []domain.TestResult{{Name: "t", Status: domain.StatusPassed}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	if bytesContains(buf.Bytes(), []byte("Authorization")) {
		t.Error("JSON output unexpectedly contains \"Authorization\"")
	}
}

func bytesContains(haystack, needle []byte) bool {
	return bytes.Contains(haystack, needle)
}
