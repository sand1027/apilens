package domain

import "time"

// TestStatus is the terminal state of one test execution.
type TestStatus string

const (
	StatusPassed  TestStatus = "passed"
	StatusFailed  TestStatus = "failed"
	StatusErrored TestStatus = "errored"
	StatusSkipped TestStatus = "skipped"
)

// TestResult is what a single TestCase produced. Reporters format this;
// they never recompute pass/fail.
type TestResult struct {
	Name   string
	File   string
	Status TestStatus
	Method Method
	URL    string
	// DisplayMethod/DisplayName are the same GraphQL-aware label
	// internal/graphqlop.DisplayColumns already computes for `watch` /
	// `history list` (e.g. "QUERY" / "Users" instead of "POST" /
	// "http://host/graphql") — populated here too so `apilens run`'s
	// terminal output shows the operation name for GraphQL tests instead
	// of every row looking like an indistinguishable "POST /graphql".
	// Both fall back to Method/URL for non-GraphQL requests, so callers
	// that only care about "some human label" never need a nil check.
	DisplayMethod string
	DisplayName   string
	HTTPStatus    int
	DurationMS    int64
	Assertions    []AssertionResult
	Error         string // populated when Status == errored
	// ResponseBody/ResponseHeaders hold the actual HTTP response this
	// test received — the same "show me the data, not just pass/fail"
	// capability Postman's collection runner has always had. Populated
	// ONLY when the caller opts in (testrunner.WithCaptureResponse /
	// `apilens run --capture-response`); empty otherwise, since a
	// response body can be large and, redacted or not, is strictly more
	// data exposure than a plain pass/fail summary. Redaction (masking
	// sensitive JSON keys, per internal/security.Redactor) is applied
	// BEFORE this field is populated — the same redacted-by-default
	// posture every other display/report path in this codebase already
	// uses (docs/09-security.md, ADR-006) — so this is never a way to
	// accidentally leak a secret into a checked-in report file that
	// wouldn't otherwise have shown up in, say, `apilens history show`.
	ResponseBody    []byte
	ResponseHeaders map[string][]string
	// CleanupResults reports the outcome of this test's cleanup: block,
	// if it had one — kept as its own field, separate from Assertions,
	// because a cleanup failure is not an assertion failure: it never
	// changes Status (a test that passed its checks but whose teardown
	// failed still Passed — the record it created just wasn't removed,
	// which is a "you may have leftover test data" warning, not a test
	// failure) and reporters must not conflate the two. Empty when the
	// test had no cleanup: block at all.
	CleanupResults []CleanupResult
}

// CleanupResult is the outcome of one cleanup.db.<connection>[i] target.
type CleanupResult struct {
	Connection string
	Collection string
	// DeletedCount is the number of documents actually removed. -1 if
	// the delete itself failed (see Error) — kept distinct from a
	// legitimate 0 (filter matched nothing, e.g. the test itself failed
	// before creating anything, so there was nothing to clean up).
	DeletedCount int
	Error        string
}

// Counts summarizes a Report for the terminal footer and JSON `counts`.
type Counts struct {
	Tests   int `json:"tests"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Errored int `json:"errored"`
	Skipped int `json:"skipped"`
}

// SuccessPercent implements docs/11-risks-and-gaps.md G15:
// passed / (passed + failed + errored), skipped excluded from denominator.
func (c Counts) SuccessPercent() float64 {
	denom := c.Passed + c.Failed + c.Errored
	if denom == 0 {
		return 0
	}
	return float64(c.Passed) / float64(denom) * 100
}

// Report is the outcome of running a suite (or a filtered subset). It is
// computed by internal/testrunner and only formatted by reporters
// (ADR-022).
type Report struct {
	Env         string
	Counts      Counts
	DurationMS  int64
	Results     []TestResult
	GeneratedAt time.Time
}

// SuiteMeta is passed to Reporter.Start before any test executes.
type SuiteMeta struct {
	Env        string
	TotalTests int
}
