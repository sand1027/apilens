package reporter

import (
	"bytes"
	"encoding/base64"
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

func TestJSONReporter_OmitsResponseFieldsWhenNotCaptured(t *testing.T) {
	// The common case: --capture-response was NOT used. ResponseBody/
	// ResponseHeaders are the zero value, and the JSON output must not
	// contain the keys at all (not even as null), matching every other
	// omitempty field in this schema.
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	report := domain.Report{
		Results: []domain.TestResult{{Name: "t", Status: domain.StatusPassed}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	if bytesContains(buf.Bytes(), []byte("response_body")) {
		t.Errorf("expected no response_body key when capture was not requested, got:\n%s", buf.String())
	}
	if bytesContains(buf.Bytes(), []byte("response_headers")) {
		t.Errorf("expected no response_headers key when capture was not requested, got:\n%s", buf.String())
	}
}

func TestJSONReporter_EmbedsJSONResponseBodyAsNativeStructure(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	report := domain.Report{
		Results: []domain.TestResult{{
			Name: "t", Status: domain.StatusPassed,
			ResponseBody:    []byte(`{"data":{"user":{"id":42,"name":"Ada"}}}`),
			ResponseHeaders: map[string][]string{"Content-Type": {"application/json"}},
		}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var decoded struct {
		Results []struct {
			ResponseBody struct {
				Data struct {
					User struct {
						ID   float64 `json:"id"`
						Name string  `json:"name"`
					} `json:"user"`
				} `json:"data"`
			} `json:"response_body"`
			ResponseHeaders map[string][]string `json:"response_headers"`
			BodyEncoding    string               `json:"body_encoding"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if len(decoded.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(decoded.Results))
	}
	r := decoded.Results[0]
	if r.ResponseBody.Data.User.ID != 42 || r.ResponseBody.Data.User.Name != "Ada" {
		t.Errorf("response_body did not decode as native JSON structure: %+v", r.ResponseBody)
	}
	if r.BodyEncoding != "" {
		t.Errorf("expected no body_encoding for a plain JSON body, got %q", r.BodyEncoding)
	}
	if len(r.ResponseHeaders["Content-Type"]) == 0 || r.ResponseHeaders["Content-Type"][0] != "application/json" {
		t.Errorf("response_headers missing Content-Type, got %v", r.ResponseHeaders)
	}
}

func TestJSONReporter_NonJSONResponseBodyEncodedAsString(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	report := domain.Report{
		Results: []domain.TestResult{{
			Name: "t", Status: domain.StatusPassed,
			ResponseBody: []byte("plain text response, not JSON"),
		}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var decoded struct {
		Results []struct {
			ResponseBody string `json:"response_body"`
			BodyEncoding string `json:"body_encoding"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded.Results[0].ResponseBody != "plain text response, not JSON" {
		t.Errorf("response_body = %q", decoded.Results[0].ResponseBody)
	}
	if decoded.Results[0].BodyEncoding != "" {
		t.Errorf("expected no body_encoding for a valid UTF-8 string body, got %q", decoded.Results[0].BodyEncoding)
	}
}

func TestJSONReporter_BinaryResponseBodyEncodedAsBase64(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	binary := []byte{0xff, 0xfe, 0x00, 0x01, 0x02, 0x80, 0x81}
	report := domain.Report{
		Results: []domain.TestResult{{
			Name: "t", Status: domain.StatusPassed,
			ResponseBody: binary,
		}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var decoded struct {
		Results []struct {
			ResponseBody string `json:"response_body"`
			BodyEncoding string `json:"body_encoding"`
		} `json:"results"`
	}
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, buf.String())
	}
	if decoded.Results[0].BodyEncoding != "base64" {
		t.Errorf("expected body_encoding=base64 for a non-UTF8 body, got %q", decoded.Results[0].BodyEncoding)
	}
	decodedBytes, err := base64.StdEncoding.DecodeString(decoded.Results[0].ResponseBody)
	if err != nil {
		t.Fatalf("response_body is not valid base64: %v", err)
	}
	if !bytes.Equal(decodedBytes, binary) {
		t.Errorf("decoded base64 = %v, want %v", decodedBytes, binary)
	}
}

func TestJSONReporter_EmptyResponseBodyOmitsField(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJSON(&buf)
	report := domain.Report{
		Results: []domain.TestResult{{Name: "t", Status: domain.StatusPassed, ResponseBody: nil}},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	if bytesContains(buf.Bytes(), []byte("response_body")) {
		t.Errorf("expected no response_body key for a nil body, got:\n%s", buf.String())
	}
}
