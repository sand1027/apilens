package reporter

import (
	"bytes"
	"errors"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

// erroringReporter always fails SuiteFinished, to test that Multi still
// calls every reporter rather than stopping at the first error.
type erroringReporter struct {
	finished bool
}

func (e *erroringReporter) Name() string                   { return "erroring" }
func (e *erroringReporter) Formats() []string              { return []string{"erroring"} }
func (e *erroringReporter) Start(domain.SuiteMeta)         {}
func (e *erroringReporter) TestFinished(domain.TestResult) {}
func (e *erroringReporter) SuiteFinished(domain.Report) error {
	e.finished = true
	return errors.New("boom")
}

func TestMulti_ForwardsStartToEveryReporter(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	m := Multi(NewTerminal(&buf1), NewTerminal(&buf2))
	m.Start(domain.SuiteMeta{TotalTests: 3})

	if !bytes.Contains(buf1.Bytes(), []byte("API TEST RESULTS")) {
		t.Error("first reporter did not receive Start")
	}
	if !bytes.Contains(buf2.Bytes(), []byte("API TEST RESULTS")) {
		t.Error("second reporter did not receive Start")
	}
}

func TestMulti_ForwardsTestFinishedToEveryReporter(t *testing.T) {
	var buf1, buf2 bytes.Buffer
	m := Multi(NewTerminal(&buf1), NewTerminal(&buf2))
	m.TestFinished(domain.TestResult{Name: "t1", Status: domain.StatusPassed, Method: "GET", URL: "/x"})

	if !bytes.Contains(buf1.Bytes(), []byte("/x")) {
		t.Error("first reporter did not receive TestFinished")
	}
	if !bytes.Contains(buf2.Bytes(), []byte("/x")) {
		t.Error("second reporter did not receive TestFinished")
	}
}

func TestMulti_SuiteFinishedCallsEveryReporterDespiteEarlierError(t *testing.T) {
	first := &erroringReporter{}
	var buf bytes.Buffer
	second := NewJSON(&buf)

	m := Multi(first, second)
	err := m.SuiteFinished(domain.Report{Counts: domain.Counts{Tests: 1, Passed: 1}})

	if err == nil {
		t.Error("expected the first reporter's error to be returned")
	}
	if !first.finished {
		t.Error("expected the erroring reporter's SuiteFinished to have run")
	}
	if buf.Len() == 0 {
		t.Error("expected the second reporter to still write its output despite the first one's error")
	}
}

func TestMulti_SetQuietForwardsToWrappedTerminalReporter(t *testing.T) {
	var buf bytes.Buffer
	term := NewTerminal(&buf)
	m := Multi(term, NewJSON(&bytes.Buffer{}))

	quieter, ok := m.(interface{ SetQuiet(bool) })
	if !ok {
		t.Fatal("Multi's result must implement SetQuiet so pkg/apilens's existing type-assertion check keeps working")
	}
	quieter.SetQuiet(true)

	m.Start(domain.SuiteMeta{})
	m.TestFinished(domain.TestResult{Name: "t1", Status: domain.StatusPassed, Method: "GET", URL: "/x"})

	if bytes.Contains(buf.Bytes(), []byte("/x")) {
		t.Error("expected SetQuiet(true) to have suppressed the passing test line in the wrapped terminal reporter")
	}
}
