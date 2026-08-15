package reporter

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestTerminalReporter_PrintsEveryResultByDefault(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminal(&buf)
	rep.Start(domain.SuiteMeta{})
	rep.TestFinished(domain.TestResult{Name: "Health", Status: domain.StatusPassed, Method: "GET", URL: "/health"})
	rep.TestFinished(domain.TestResult{Name: "Broken", Status: domain.StatusFailed, Method: "GET", URL: "/broken"})

	out := buf.String()
	if !strings.Contains(out, "/health") {
		t.Error("expected passing test line to be printed by default")
	}
	if !strings.Contains(out, "/broken") {
		t.Error("expected failing test line to be printed")
	}
}

func TestTerminalReporter_QuietSuppressesPassingAndSkipped(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminal(&buf)
	rep.SetQuiet(true)
	rep.Start(domain.SuiteMeta{})
	rep.TestFinished(domain.TestResult{Name: "Health", Status: domain.StatusPassed, Method: "GET", URL: "/health"})
	rep.TestFinished(domain.TestResult{Name: "Skipped", Status: domain.StatusSkipped, Method: "GET", URL: "/skipped"})
	rep.TestFinished(domain.TestResult{Name: "Broken", Status: domain.StatusFailed, Method: "GET", URL: "/broken"})
	rep.TestFinished(domain.TestResult{Name: "Errored", Status: domain.StatusErrored, Method: "GET", URL: "/errored", Error: "boom"})

	out := buf.String()
	if strings.Contains(out, "/health") {
		t.Error("--quiet should suppress a passing test line")
	}
	if strings.Contains(out, "/skipped") {
		t.Error("--quiet should suppress a skipped test line")
	}
	if !strings.Contains(out, "/broken") {
		t.Error("--quiet must still print a failing test line")
	}
	if !strings.Contains(out, "/errored") {
		t.Error("--quiet must still print an errored test line")
	}
}

func TestTerminalReporter_QuietStillPrintsSummary(t *testing.T) {
	var buf bytes.Buffer
	rep := NewTerminal(&buf)
	rep.SetQuiet(true)
	report := domain.Report{Counts: domain.Counts{Tests: 4, Passed: 3, Failed: 1}}
	if err := rep.SuiteFinished(report); err != nil {
		t.Fatalf("SuiteFinished: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "Tests:    4") {
		t.Errorf("expected summary counts even with --quiet, got: %s", out)
	}
}
