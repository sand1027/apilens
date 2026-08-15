package cli

import "github.com/sandeepv/apilens/pkg/apilens"

// exitCode is set by command handlers (run/test) that need to distinguish
// "usage error" (2) from "tests ran but some failed" (1) without treating
// the latter as a cobra RunE error (which would also print a stray
// "Error:" line — docs/05-cli.md section 4 exit codes are a report
// outcome, not a Go error). main.go reads this after Execute() returns.
var lastExitCode int

func setExitCode(code int) {
	lastExitCode = code
}

// LastExitCode returns the exit code recorded by the most recent command,
// defaulting to ExitOK if no command set one explicitly.
func LastExitCode() int {
	return lastExitCode
}

func exitCodeForReport(report apilens.Report) int {
	if report.Counts.Failed > 0 || report.Counts.Errored > 0 {
		return ExitTestFailure
	}
	return ExitOK
}
