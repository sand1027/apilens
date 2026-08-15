// Package testrunner orchestrates running a suite of domain.TestCase:
// sequential or parallel, retries on transport errors only, per-test
// timeout, fail-fast. It computes the domain.Report; reporters only format
// it (ADR-022). See docs/04-interfaces.md section 8.
package testrunner

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/sandeepv/apilens/internal/assertions"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/environment"
	"github.com/sandeepv/apilens/internal/reporter"
	"github.com/sandeepv/apilens/internal/runner"
)

// Options mirrors docs/04-interfaces.md section 8.
type Options struct {
	Parallel bool
	Workers  int
	Retries  int // suite-level default; a TestCase.Retries of its own wins if set explicitly
	Timeout  time.Duration
	FailFast bool
}

// Runner executes a suite of compiled tests against a resolved environment.
type Runner struct {
	http   *runner.Runner
	env    *environment.Resolver
	assert *assertions.Engine
	rep    reporter.Reporter
}

// New builds a testrunner.Runner. rep may be nil if the caller only needs
// the Report and does not want incremental output.
func New(httpRunner *runner.Runner, env *environment.Resolver, assertEngine *assertions.Engine, rep reporter.Reporter) *Runner {
	return &Runner{http: httpRunner, env: env, assert: assertEngine, rep: rep}
}

// Run executes every test in tests according to opts and returns the
// computed Report. If opts.FailFast is set, remaining tests are marked
// skipped once the first failure/error occurs (sequential mode only —
// docs/05-cli.md section 3 lists --fail-fast as a run flag).
func (r *Runner) Run(ctx context.Context, tests []domain.TestCase, opts Options) (domain.Report, error) {
	if len(tests) == 0 {
		return domain.Report{}, domain.NewConfigError("no tests to run", nil)
	}

	if r.rep != nil {
		r.rep.Start(domain.SuiteMeta{Env: string(r.env.CurrentName()), TotalTests: len(tests)})
	}

	// Clear any responses recorded by a previous Run on this same
	// Resolver (e.g. the web dashboard reuses one Engine/Resolver across
	// many `apilens ui` requests) — a chained suite must never resolve
	// against a stale response from an earlier, unrelated run (DSL v2
	// chaining, plan.md v9).
	r.env.ResetResponses()

	start := time.Now()
	var results []domain.TestResult
	if opts.Parallel {
		results = r.runParallel(ctx, tests, opts)
	} else {
		results = r.runSequential(ctx, tests, opts)
	}
	duration := time.Since(start)

	report := buildReport(string(r.env.CurrentName()), results, duration)
	if r.rep != nil {
		if err := r.rep.SuiteFinished(report); err != nil {
			return report, domain.NewConfigError("writing report", err)
		}
	}
	return report, nil
}

func (r *Runner) runSequential(ctx context.Context, tests []domain.TestCase, opts Options) []domain.TestResult {
	results := make([]domain.TestResult, 0, len(tests))
	failed := false
	for _, tc := range tests {
		var result domain.TestResult
		if failed && opts.FailFast {
			result = skippedResult(tc)
		} else {
			result = r.runOne(ctx, tc, opts)
			if result.Status == domain.StatusFailed || result.Status == domain.StatusErrored {
				failed = true
			}
		}
		if r.rep != nil {
			r.rep.TestFinished(result)
		}
		results = append(results, result)
	}
	return results
}

func (r *Runner) runParallel(ctx context.Context, tests []domain.TestCase, opts Options) []domain.TestResult {
	workers := opts.Workers
	if workers <= 0 {
		workers = runtime.GOMAXPROCS(0)
	}
	if workers > len(tests) {
		workers = len(tests)
	}

	results := make([]domain.TestResult, len(tests))
	jobs := make(chan int)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				// Each test gets its own copy of the RequestTemplate (via
				// value semantics of domain.TestCase) — no shared mutable
				// request structs across goroutines (docs/11-risks-and-gaps.md R6).
				result := r.runOne(ctx, tests[idx], opts)
				mu.Lock()
				results[idx] = result
				mu.Unlock()
				if r.rep != nil {
					r.rep.TestFinished(result)
				}
			}
		}()
	}
	for i := range tests {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return results
}

func skippedResult(tc domain.TestCase) domain.TestResult {
	return domain.TestResult{
		Name:   tc.Name,
		File:   tc.File,
		Status: domain.StatusSkipped,
		Method: tc.Request.Method,
	}
}

// runOne executes a single test: interpolate, execute (with retry on
// transport errors only), assert. Compile errors never reach here — testdef
// already compiled the assertion spec at load time; here we only compile it
// into executable checks.
func (r *Runner) runOne(ctx context.Context, tc domain.TestCase, opts Options) domain.TestResult {
	base := domain.TestResult{Name: tc.Name, File: tc.File, Method: tc.Request.Method}

	if tc.Skip {
		base.Status = domain.StatusSkipped
		return base
	}

	checks, err := r.assert.Compile(tc.Assert)
	if err != nil {
		base.Status = domain.StatusErrored
		base.Error = err.Error()
		return base
	}

	req, err := r.env.InterpolateRequest(tc.Request)
	if err != nil {
		base.Status = domain.StatusErrored
		base.Error = err.Error()
		return base
	}
	base.URL = req.URL

	timeout := tc.Timeout
	if timeout <= 0 {
		timeout = opts.Timeout
	}
	if timeout > 0 {
		req.Timeout = timeout
	}

	retries := opts.Retries
	if tc.Retries > 0 {
		retries = tc.Retries
	}
	if retries < 0 {
		retries = 0
	}

	var ex domain.Exchange
	var lastErr error
	attempts := retries + 1
	for attempt := 0; attempt < attempts; attempt++ {
		ex, err = r.http.Do(ctx, req)
		if err == nil {
			lastErr = nil
			break
		}
		lastErr = err
		if !errors.Is(err, domain.ErrTransport) {
			break // non-transport errors (e.g. bad URL) do not retry
		}
	}
	if lastErr != nil {
		base.Status = domain.StatusErrored
		base.Error = lastErr.Error()
		base.DurationMS = ex.Timing.Duration.Milliseconds()
		// Still record on error: a chained test referencing "{{responses.
		// login.status}}" should see the real (failing) status rather
		// than nothing at all — the reference itself only ever fails if
		// this test never ran, not if it ran and errored.
		if tc.ID != "" {
			r.env.RecordResponse(tc.ID, ex)
		}
		return base
	}

	base.HTTPStatus = ex.Response.StatusCode
	base.DurationMS = ex.Timing.Duration.Milliseconds()

	// Record BEFORE evaluating assertions so a later chained test can use
	// this response even if this test's own assertions fail — chaining is
	// about response data flow, not about gating on pass/fail (DSL v2,
	// plan.md v9).
	if tc.ID != "" {
		r.env.RecordResponse(tc.ID, ex)
	}

	results := r.assert.Eval(checks, ex)
	base.Assertions = results
	base.Status = domain.StatusPassed
	for _, a := range results {
		if !a.Passed {
			base.Status = domain.StatusFailed
			break
		}
	}
	return base
}

func buildReport(env string, results []domain.TestResult, duration time.Duration) domain.Report {
	counts := domain.Counts{Tests: len(results)}
	for _, r := range results {
		switch r.Status {
		case domain.StatusPassed:
			counts.Passed++
		case domain.StatusFailed:
			counts.Failed++
		case domain.StatusErrored:
			counts.Errored++
		case domain.StatusSkipped:
			counts.Skipped++
		}
	}
	return domain.Report{
		Env:         env,
		Counts:      counts,
		DurationMS:  duration.Milliseconds(),
		Results:     results,
		GeneratedAt: time.Now(),
	}
}

// ExitCode maps a Report to the process exit code per docs/05-cli.md
// section 4: 0 all passed/skipped-only, 1 any failed/errored.
func ExitCode(report domain.Report) int {
	if report.Counts.Failed > 0 || report.Counts.Errored > 0 {
		return 1
	}
	return 0
}
