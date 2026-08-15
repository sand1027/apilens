package reporter

import (
	"bytes"
	"encoding/xml"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestJUnitReporter_ProducesValidXML(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)

	report := domain.Report{
		Env: "local",
		Counts: domain.Counts{
			Tests: 3, Passed: 1, Failed: 1, Errored: 1, Skipped: 0,
		},
		DurationMS: 15,
		Results: []domain.TestResult{
			{Name: "Health", File: "smoke/health.yaml", Status: domain.StatusPassed, DurationMS: 5},
			{
				Name: "Get User", File: "users/get-user.yaml", Status: domain.StatusFailed, DurationMS: 6,
				Assertions: []domain.AssertionResult{
					{Kind: domain.KindStatusEquals, Passed: false, Expected: "200", Actual: "500"},
				},
			},
			{Name: "List Users", File: "users/list.yaml", Status: domain.StatusErrored, DurationMS: 4, Error: "connection refused"},
		},
	}

	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var doc junitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("output is not valid XML: %v\n%s", err, buf.String())
	}
	if len(doc.Suites) != 1 {
		t.Fatalf("expected 1 suite, got %d", len(doc.Suites))
	}
	suite := doc.Suites[0]
	if suite.Tests != 3 || suite.Failures != 1 || suite.Errors != 1 {
		t.Errorf("suite counts = %+v", suite)
	}
	if len(suite.TestCases) != 3 {
		t.Fatalf("expected 3 testcases, got %d", len(suite.TestCases))
	}
}

func TestJUnitReporter_FailureHasMessageAndContent(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)
	report := domain.Report{
		Results: []domain.TestResult{
			{
				Name: "Get User", Status: domain.StatusFailed,
				Assertions: []domain.AssertionResult{
					{Kind: domain.KindStatusEquals, Passed: false, Expected: "200", Actual: "500"},
				},
			},
		},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}

	var doc junitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	tc := doc.Suites[0].TestCases[0]
	if tc.Failure == nil {
		t.Fatal("expected a <failure> element")
	}
	if tc.Failure.Message == "" {
		t.Error("expected a non-empty failure message attribute")
	}
	if !containsSubstr(tc.Failure.Content, "expected 200") {
		t.Errorf("failure content = %q, want it to mention the assertion detail", tc.Failure.Content)
	}
}

func TestJUnitReporter_ErrorHasErrorElement(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)
	report := domain.Report{
		Results: []domain.TestResult{
			{Name: "List Users", Status: domain.StatusErrored, Error: "connection refused"},
		},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	var doc junitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	tc := doc.Suites[0].TestCases[0]
	if tc.Error == nil {
		t.Fatal("expected an <error> element")
	}
	if tc.Error.Message != "connection refused" {
		t.Errorf("error message = %q", tc.Error.Message)
	}
	if tc.Failure != nil {
		t.Error("an errored test must not also have a <failure> element")
	}
}

func TestJUnitReporter_SkippedHasSkippedElement(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)
	report := domain.Report{
		Results: []domain.TestResult{
			{Name: "Maybe Later", Status: domain.StatusSkipped},
		},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	var doc junitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	tc := doc.Suites[0].TestCases[0]
	if tc.Skipped == nil {
		t.Fatal("expected a <skipped> element")
	}
}

func TestJUnitReporter_PassedTestHasNoFailureErrorOrSkipped(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)
	report := domain.Report{
		Results: []domain.TestResult{
			{Name: "Health", Status: domain.StatusPassed},
		},
	}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	var doc junitTestSuites
	if err := xml.Unmarshal(buf.Bytes(), &doc); err != nil {
		t.Fatalf("invalid XML: %v", err)
	}
	tc := doc.Suites[0].TestCases[0]
	if tc.Failure != nil || tc.Error != nil || tc.Skipped != nil {
		t.Errorf("passed test should have no failure/error/skipped elements, got %+v", tc)
	}
}

func TestJUnitReporter_NeverContainsRawAuthorizationValue(t *testing.T) {
	var buf bytes.Buffer
	rep := NewJUnit(&buf)
	report := domain.Report{Results: []domain.TestResult{{Name: "t", Status: domain.StatusPassed}}}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	if bytes.Contains(buf.Bytes(), []byte("Authorization")) {
		t.Error("JUnit output unexpectedly contains \"Authorization\"")
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
