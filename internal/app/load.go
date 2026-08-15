package app

import (
	"context"
	"path/filepath"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/loadtest"
	"github.com/sandeepv/apilens/internal/runner"
	"github.com/sandeepv/apilens/internal/testdef"
)

// LoadOptions configures RunLoad (plan.md v9: "Load / soak mode
// (apilens run --load) on the same runner").
type LoadOptions struct {
	Duration   time.Duration
	Iterations int
	Workers    int
	Timeout    time.Duration
}

// RunLoad loads and filters tests exactly like RunSuite (same loader,
// same RunFilter semantics — a load test targets the same test set an
// ordinary run would, per plan.md v9's "on the same runner"), then
// repeats them via internal/loadtest instead of testrunner. Chained (DSL
// v2) tests are rejected up front with a clear message rather than
// silently racing.
func (a *App) RunLoad(ctx context.Context, filter RunFilter, loadOpts LoadOptions) (loadtest.Report, error) {
	testsDir := filepath.Join(a.ProjectDir, ".apilens", "tests")
	loader := testdef.NewLoader()
	tests, err := loader.LoadAll(testsDir)
	if err != nil {
		return loadtest.Report{}, err
	}

	tests = filterTests(tests, filter)
	if len(tests) == 0 {
		return loadtest.Report{}, domain.NewConfigError(
			"no tests matched — write a YAML test under .apilens/tests or run apilens generate", nil)
	}

	httpRunner := runner.New(runner.WithMaxResponseSize(a.Config.MaxResponseSizeBytes()))
	assertEngine, closeDB := a.buildAssertEngine()
	defer closeDB()

	return loadtest.Run(ctx, tests, httpRunner, a.Env, assertEngine, loadtest.Options{
		Duration:   loadOpts.Duration,
		Iterations: loadOpts.Iterations,
		Workers:    loadOpts.Workers,
		Timeout:    loadOpts.Timeout,
	})
}
