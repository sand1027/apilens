// Package loadtest implements plan.md v9's "Load / soak mode (`apilens
// run --load`) on the same runner": it repeats a filtered set of already-
// compiled domain.TestCase against the shared internal/runner.Runner and
// internal/environment.Resolver, across concurrent workers, for a bounded
// duration or iteration count, and reports latency percentiles plus
// throughput/error-rate — not pass/fail. It deliberately reuses the exact
// runner/environment/assertion path a normal `apilens run` uses (per
// plan.md's "without becoming a separate load-test company": no second
// HTTP client, no second interpolation engine).
package loadtest

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sandeepv/apilens/internal/assertions"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/environment"
	"github.com/sandeepv/apilens/internal/runner"
)

// Options configures Run. Exactly one of Duration/Iterations must be
// positive (both may be set — whichever bound is reached first stops the
// run); requiring at least one prevents an accidental unbounded load
// generator.
type Options struct {
	Duration   time.Duration // wall-clock bound; 0 means "no duration bound"
	Iterations int           // total-iterations-across-all-workers bound; 0 means "no iteration bound"
	Workers    int           // concurrent workers; <=0 defaults to 10
	Timeout    time.Duration // per-request timeout override; <=0 uses each TestCase's own
}

// TestStats aggregates every request made against one TestCase during the
// run.
type TestStats struct {
	Name        string
	Requests    int
	Errors      int // transport errors OR non-2xx/3xx status codes
	Percentiles Percentiles
}

// Report is loadtest's result shape — deliberately distinct from
// domain.Report (which is pass/fail-oriented and doesn't fit a
// repeated-execution model: there is no single "Counts.Passed" for a
// request repeated 10,000 times).
type Report struct {
	StartedAt     time.Time
	DurationMS    int64
	Workers       int
	TotalRequests int
	TotalErrors   int
	Overall       Percentiles
	PerTest       []TestStats
}

// ErrorRate returns TotalErrors/TotalRequests, or 0 if no requests were
// made.
func (r Report) ErrorRate() float64 {
	if r.TotalRequests == 0 {
		return 0
	}
	return float64(r.TotalErrors) / float64(r.TotalRequests)
}

// sample is one completed request, tagged with which TestCase it came
// from (by index into the tests slice, not by pointer/name, so aggregation
// can use a plain slice instead of a map keyed by a possibly-empty Name).
type sample struct {
	testIdx  int
	duration time.Duration
	isError  bool
}

// Run repeats tests across opts.Workers concurrent workers until
// opts.Duration elapses and/or opts.Iterations total iterations complete
// (whichever comes first), or ctx is canceled. Each worker cycles through
// tests in order, interpolating and executing each one via the shared
// http/env/assert stack — reusing runner.Runner.Do and
// environment.Resolver.Interpolate exactly as testrunner does, so load
// mode measures the real request path, not a synthetic shortcut.
func Run(ctx context.Context, tests []domain.TestCase, httpRunner *runner.Runner, env *environment.Resolver, assertEngine *assertions.Engine, opts Options) (Report, error) {
	if len(tests) == 0 {
		return Report{}, domain.NewConfigError("no tests to load test", nil)
	}
	if opts.Duration <= 0 && opts.Iterations <= 0 {
		return Report{}, domain.NewConfigError(
			"load mode requires --duration and/or --iterations (both zero would run forever)", nil)
	}
	for _, tc := range tests {
		if tc.UsesChaining {
			return Report{}, domain.NewConfigError(
				fmt.Sprintf("%s: DSL v2 chained tests cannot run in load mode — "+
					"concurrently repeating a chained flow races on the shared response store; "+
					"exclude chained tests from the load-mode filter", tc.Name), nil)
		}
	}

	workers := opts.Workers
	if workers <= 0 {
		workers = 10
	}

	runCtx := ctx
	var cancel context.CancelFunc
	if opts.Duration > 0 {
		runCtx, cancel = context.WithTimeout(ctx, opts.Duration)
		defer cancel()
	}

	if assertEngine == nil {
		assertEngine = assertions.New()
	}
	// Assertion checks are compiled once per test up front (outside the
	// hot loop) — every worker shares the same compiled
	// domain.AssertionSet, which is read-only after Compile, so this is
	// safe to reuse concurrently.
	checks := make([]domain.AssertionSet, len(tests))
	for i, tc := range tests {
		set, err := assertEngine.Compile(tc.Assert)
		if err != nil {
			return Report{}, err
		}
		checks[i] = set
	}

	hasIterationBound := opts.Iterations > 0
	var remaining int64
	if hasIterationBound {
		remaining = int64(opts.Iterations)
	}

	samplesCh := make(chan sample, workers*4)
	var wg sync.WaitGroup
	start := time.Now()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runWorker(runCtx, tests, checks, httpRunner, env, assertEngine, opts, hasIterationBound, &remaining, samplesCh)
		}()
	}

	var collected []sample
	done := make(chan struct{})
	go func() {
		for s := range samplesCh {
			collected = append(collected, s)
		}
		close(done)
	}()

	wg.Wait()
	close(samplesCh)
	<-done

	duration := time.Since(start)
	return buildReport(start, duration, workers, tests, collected), nil
}

// runWorker executes tests in a round-robin cycle until the duration
// deadline (via runCtx.Done()) or the shared iteration budget is
// exhausted, sending one sample per completed request.
func runWorker(
	runCtx context.Context,
	tests []domain.TestCase,
	checks []domain.AssertionSet,
	httpRunner *runner.Runner,
	env *environment.Resolver,
	assertEngine *assertions.Engine,
	opts Options,
	hasIterationBound bool,
	remaining *int64,
	out chan<- sample,
) {
	i := 0
	for {
		select {
		case <-runCtx.Done():
			return
		default:
		}
		// hasIterationBound is fixed for the whole run — unlike the
		// counter itself, it must NOT be inferred from remaining's
		// current value, since remaining legitimately becomes negative
		// once budget is exhausted (see below) and a naive "is it
		// negative" check would then mistake "exhausted" for
		// "unbounded" and spin forever.
		if hasIterationBound {
			// Every worker atomically decrements the same shared budget;
			// the first to observe it go negative stops. This keeps the
			// total iteration count across all workers at exactly
			// opts.Iterations even though decrements race.
			if atomic.AddInt64(remaining, -1) < 0 {
				return
			}
		}

		idx := i % len(tests)
		i++
		tc := tests[idx]

		req, err := env.InterpolateRequest(tc.Request)
		if err != nil {
			out <- sample{testIdx: idx, isError: true}
			continue
		}
		if opts.Timeout > 0 {
			req.Timeout = opts.Timeout
		}

		reqStart := time.Now()
		ex, err := httpRunner.Do(runCtx, req)
		elapsed := time.Since(reqStart)

		isError := err != nil
		if !isError {
			results := assertEngine.Eval(checks[idx], ex)
			for _, a := range results {
				if !a.Passed {
					isError = true
					break
				}
			}
		}
		out <- sample{testIdx: idx, duration: elapsed, isError: isError}
	}
}

func buildReport(start time.Time, duration time.Duration, workers int, tests []domain.TestCase, samples []sample) Report {
	perTestDurations := make([][]time.Duration, len(tests))
	perTestErrors := make([]int, len(tests))
	var allDurations []time.Duration
	totalErrors := 0

	for _, s := range samples {
		if s.isError {
			totalErrors++
			perTestErrors[s.testIdx]++
			continue // an errored sample has no meaningful latency to aggregate
		}
		perTestDurations[s.testIdx] = append(perTestDurations[s.testIdx], s.duration)
		allDurations = append(allDurations, s.duration)
	}

	perTest := make([]TestStats, len(tests))
	for i, tc := range tests {
		perTest[i] = TestStats{
			Name:        tc.Name,
			Requests:    len(perTestDurations[i]) + perTestErrors[i],
			Errors:      perTestErrors[i],
			Percentiles: computePercentiles(perTestDurations[i]),
		}
	}

	return Report{
		StartedAt:     start,
		DurationMS:    duration.Milliseconds(),
		Workers:       workers,
		TotalRequests: len(samples),
		TotalErrors:   totalErrors,
		Overall:       computePercentiles(allDurations),
		PerTest:       perTest,
	}
}
