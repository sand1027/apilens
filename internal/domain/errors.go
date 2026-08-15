package domain

import "errors"

// Cross-cutting error classes. Every use case and CLI command maps errors to
// an exit code by checking errors.Is against these sentinels (or wrapping
// one of the *Error types below). See docs/04-interfaces.md section 15.
var (
	// ErrConfig covers bad YAML, missing/unknown format, missing variables,
	// unknown assertion kinds, zero tests to run. Exit code 2.
	ErrConfig = errors.New("config error")

	// ErrNotFound covers unknown test, endpoint, history id, environment.
	// Exit code 2.
	ErrNotFound = errors.New("not found")

	// ErrSecurity covers bind policy rejection, refused secret handling.
	// Exit code 2.
	ErrSecurity = errors.New("security error")

	// ErrTransport covers network-level failures after retries are
	// exhausted. Inside a suite this becomes a TestResult with
	// StatusErrored; exit code 1 if it's part of a suite run.
	ErrTransport = errors.New("transport error")

	// ErrNotImplemented is returned by Engine methods not yet implemented
	// in the current product version (e.g. Watch/Replay/Generate in v1).
	ErrNotImplemented = errors.New("not implemented")
)

// ConfigError wraps ErrConfig with a human-readable message and keeps the
// original cause (if any) available via Unwrap.
type ConfigError struct {
	Msg   string
	Cause error
}

func (e *ConfigError) Error() string {
	if e.Cause != nil {
		return e.Msg + ": " + e.Cause.Error()
	}
	return e.Msg
}

func (e *ConfigError) Unwrap() error { return ErrConfig }

// NewConfigError builds a ConfigError.
func NewConfigError(msg string, cause error) *ConfigError {
	return &ConfigError{Msg: msg, Cause: cause}
}

// NotFoundError wraps ErrNotFound with a human-readable message.
type NotFoundError struct {
	Msg string
}

func (e *NotFoundError) Error() string { return e.Msg }
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// NewNotFoundError builds a NotFoundError.
func NewNotFoundError(msg string) *NotFoundError {
	return &NotFoundError{Msg: msg}
}

// SecurityError wraps ErrSecurity with a human-readable message.
type SecurityError struct {
	Msg string
}

func (e *SecurityError) Error() string { return e.Msg }
func (e *SecurityError) Unwrap() error { return ErrSecurity }

// NewSecurityError builds a SecurityError.
func NewSecurityError(msg string) *SecurityError {
	return &SecurityError{Msg: msg}
}
