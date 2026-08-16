package reporter

import "github.com/sandeepv/apilens/internal/domain"

// multiReporter fans out every call to a fixed set of Reporters, so a
// single testrunner.Run can drive both a stdout-facing reporter and a
// file-writing one at once (`apilens run --out report.json`, still
// printing the normal terminal output too) without testrunner itself
// needing to know there is more than one destination.
type multiReporter struct {
	reporters []Reporter
}

// Multi builds a Reporter that forwards every call to each of reps, in
// order. It is never looked up by name through Registry (Name/Formats
// exist only for interface completeness) — callers construct it directly
// once they already know which concrete reporters they want combined.
func Multi(reps ...Reporter) Reporter {
	return &multiReporter{reporters: reps}
}

func (m *multiReporter) Name() string {
	if len(m.reporters) == 0 {
		return "multi"
	}
	return m.reporters[0].Name()
}

func (m *multiReporter) Formats() []string {
	if len(m.reporters) == 0 {
		return nil
	}
	return m.reporters[0].Formats()
}

func (m *multiReporter) Start(meta domain.SuiteMeta) {
	for _, r := range m.reporters {
		r.Start(meta)
	}
}

func (m *multiReporter) TestFinished(result domain.TestResult) {
	for _, r := range m.reporters {
		r.TestFinished(result)
	}
}

// SuiteFinished calls every reporter's SuiteFinished even if an earlier
// one errors, so a failure writing the file report can never suppress
// the terminal summary the user is watching live. Returns the first
// error encountered (if any) after every reporter has had a chance to
// run, matching the single-error Reporter contract.
func (m *multiReporter) SuiteFinished(report domain.Report) error {
	var firstErr error
	for _, r := range m.reporters {
		if err := r.SuiteFinished(report); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// SetQuiet forwards to every wrapped reporter that supports it (only
// TerminalReporter does today). This lets pkg/apilens's existing
// "if quieter, ok := rep.(interface{ SetQuiet(bool) })" check keep
// working transparently even when rep is actually a multiReporter wrapping
// a terminal reporter alongside a file-writing one.
func (m *multiReporter) SetQuiet(quiet bool) {
	for _, r := range m.reporters {
		if quieter, ok := r.(interface{ SetQuiet(bool) }); ok {
			quieter.SetQuiet(quiet)
		}
	}
}
