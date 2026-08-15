package reporter

import (
	"fmt"
	"io"
	"os"

	"github.com/sandeepv/apilens/internal/domain"
)

// TerminalReporter writes the human-readable output described in
// docs/05-cli.md section 3 ("apilens run" sample output). It writes
// incrementally as each test finishes (docs/04-interfaces.md section 9).
type TerminalReporter struct {
	out   io.Writer
	quiet bool
}

// NewTerminal builds a TerminalReporter writing to w (os.Stdout in the CLI).
func NewTerminal(w io.Writer) *TerminalReporter {
	if w == nil {
		w = os.Stdout
	}
	return &TerminalReporter{out: w}
}

// SetQuiet implements docs/05-cli.md's global `--quiet` flag ("Errors
// only"): passing/skipped test lines are suppressed, only failed/errored
// tests print, and the final summary always prints so CI logs still show
// counts and the exit-code reason (docs/10-plan.md section 7:
// "--quiet / machine-friendly logs").
func (t *TerminalReporter) SetQuiet(quiet bool) {
	t.quiet = quiet
}

func (t *TerminalReporter) Name() string      { return "terminal" }
func (t *TerminalReporter) Formats() []string { return []string{"terminal"} }

func (t *TerminalReporter) Start(meta domain.SuiteMeta) {
	fmt.Fprintln(t.out, "API TEST RESULTS")
	fmt.Fprintln(t.out)
}

func (t *TerminalReporter) TestFinished(result domain.TestResult) {
	if t.quiet && result.Status != domain.StatusFailed && result.Status != domain.StatusErrored {
		return
	}
	mark := statusMark(result.Status)
	fmt.Fprintf(t.out, "%s %-6s %-24s %-6d %dms\n",
		mark, result.Method, result.URL, result.HTTPStatus, result.DurationMS)
	if result.Status == domain.StatusFailed {
		for _, a := range result.Assertions {
			if !a.Passed {
				fmt.Fprintf(t.out, "    %s: expected %s, got %s%s\n",
					a.Kind, a.Expected, a.Actual, reasonSuffix(a.Reason))
			}
		}
	}
	if result.Status == domain.StatusErrored {
		fmt.Fprintf(t.out, "    error: %s\n", result.Error)
	}
}

func reasonSuffix(reason string) string {
	if reason == "" {
		return ""
	}
	return " (" + reason + ")"
}

func statusMark(s domain.TestStatus) string {
	switch s {
	case domain.StatusPassed:
		return "\u2713"
	case domain.StatusSkipped:
		return "\u25cb"
	default:
		return "\u2717"
	}
}

func (t *TerminalReporter) SuiteFinished(report domain.Report) error {
	fmt.Fprintln(t.out)
	fmt.Fprintf(t.out, "Tests:    %d\n", report.Counts.Tests)
	fmt.Fprintf(t.out, "Passed:   %d\n", report.Counts.Passed)
	fmt.Fprintf(t.out, "Failed:   %d\n", report.Counts.Failed)
	if report.Counts.Errored > 0 {
		fmt.Fprintf(t.out, "Errored:  %d\n", report.Counts.Errored)
	}
	if report.Counts.Skipped > 0 {
		fmt.Fprintf(t.out, "Skipped:  %d\n", report.Counts.Skipped)
	}
	fmt.Fprintf(t.out, "Success:  %.0f%%\n", report.Counts.SuccessPercent())
	return nil
}
