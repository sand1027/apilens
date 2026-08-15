// Package cli implements the Cobra command tree. It is a thin adapter over
// pkg/apilens.Engine: flags -> Engine -> print (docs/02-packages.md
// section 4, docs/05-cli.md). No command reaches internal/runner,
// internal/assertions, or internal/testrunner directly.
package cli

import (
	"errors"

	"github.com/sandeepv/apilens/internal/domain"
)

// Exit codes per docs/05-cli.md section 4.
const (
	ExitOK          = 0
	ExitTestFailure = 1
	ExitUsageError  = 2
)

// ExitCodeForError maps an error returned by an Engine call to a process
// exit code. All commands funnel through this one function
// (docs/04-interfaces.md section 15: "CLI maps these in one place").
func ExitCodeForError(err error) int {
	if err == nil {
		return ExitOK
	}
	switch {
	case errors.Is(err, domain.ErrConfig),
		errors.Is(err, domain.ErrNotFound),
		errors.Is(err, domain.ErrSecurity),
		errors.Is(err, domain.ErrNotImplemented):
		return ExitUsageError
	case errors.Is(err, domain.ErrTransport):
		return ExitTestFailure
	default:
		return ExitUsageError
	}
}
