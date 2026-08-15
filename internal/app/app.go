// Package app implements the use-case layer (docs/01-architecture.md
// section 6). Use cases orchestrate domain + core packages; they do not
// parse flags or know about YAML schema details (docs/02-packages.md
// section 3).
package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandeepv/apilens/internal/assertions"
	"github.com/sandeepv/apilens/internal/config"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/environment"
	"github.com/sandeepv/apilens/internal/history"
	"github.com/sandeepv/apilens/internal/plugins"
	"github.com/sandeepv/apilens/internal/project"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/reporter"
	"github.com/sandeepv/apilens/internal/runner"
	"github.com/sandeepv/apilens/internal/testdef"
	"github.com/sandeepv/apilens/internal/testrunner"
)

// App wires together the packages needed by each shipped use case: Init
// and RunSuite (v1), Discover/List/Inspect (v2), Watch/History (v3). Later
// use cases (Replay, Generate) are added to this struct in their
// respective versions without changing its shape for existing callers
// (docs/01-architecture.md section 6 phase column).
type App struct {
	ProjectDir string
	Config     config.Config
	Env        *environment.Resolver
	Host       *plugins.Host
	Registry   registry.Store
	History    history.Store
}

// Options configures New.
type Options struct {
	ProjectDir string
	ConfigPath string // defaults to <ProjectDir>/.apilens/config.yaml
	EnvDir     string // defaults to <ProjectDir>/.apilens/environments
}

// New loads config and environments and builds an App ready for RunSuite.
// It does not require .apilens/tests to exist yet — that's only checked
// when actually running a suite (ErrConfig on zero tests, per
// docs/11-risks-and-gaps.md G16).
func New(opts Options) (*App, error) {
	if opts.ProjectDir == "" {
		opts.ProjectDir = "."
	}
	if opts.ConfigPath == "" {
		opts.ConfigPath = filepath.Join(opts.ProjectDir, ".apilens", "config.yaml")
	}
	if opts.EnvDir == "" {
		opts.EnvDir = filepath.Join(opts.ProjectDir, ".apilens", "environments")
	}

	cfg, err := config.Load(opts.ConfigPath)
	if err != nil {
		return nil, err
	}

	env := environment.New()
	if err := env.LoadDir(opts.EnvDir); err != nil {
		return nil, err
	}

	host := plugins.NewHost()
	plugins.RegisterBuiltins(host, reporter.NewTerminal(nil), reporter.NewJSON(nil))

	reg := registry.NewMemoryStore()
	// Hydrate from the last discover write so `list`/`inspect` work in a
	// new process (docs/07-discovery.md section 8, ADR-017). A missing
	// file just means "run discover first" — not an error.
	if cached, err := registry.LoadYAML(registryPath(opts.ProjectDir)); err == nil && len(cached) > 0 {
		_ = reg.Replace(cached)
	}

	a := &App{
		ProjectDir: opts.ProjectDir,
		Config:     cfg,
		Env:        env,
		Host:       host,
		Registry:   reg,
		History:    history.NewMemoryStore(0),
	}

	// Restore the previously `env use`-selected environment, if any
	// (docs/05-cli.md section 3: "use writes .apilens/.current-env
	// (gitignored)"). A missing/unreadable file, or an env name that no
	// longer exists, is not fatal — fall back to "local" if present.
	if name := a.readCurrentEnvFile(); name != "" {
		_ = a.Env.Use(domain.EnvName(name))
	} else if _, ok := a.Env.Get("local"); ok {
		_ = a.Env.Use("local")
	}

	return a, nil
}

func (a *App) currentEnvFilePath() string {
	return filepath.Join(a.ProjectDir, ".apilens", ".current-env")
}

func registryPath(projectDir string) string {
	return filepath.Join(projectDir, ".apilens", "api", "registry.yaml")
}

func (a *App) readCurrentEnvFile() string {
	data, err := os.ReadFile(a.currentEnvFilePath())
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

// Init implements the `apilens init` use case.
func Init(projectDir string, force bool) (project.Result, error) {
	return project.Init(projectDir, force)
}

// UseEnv selects the active environment by name for the current process
// only. It does not persist — that is a separate, explicit step
// (PersistCurrentEnv) so that a one-off "--env" flag on any command does
// not change the project's persisted default (docs/05-cli.md section 2:
// "--env on any command overrides it for that invocation").
func (a *App) UseEnv(name string) error {
	return a.Env.Use(domain.EnvName(name))
}

// PersistCurrentEnv writes the currently selected environment to
// .apilens/.current-env so later invocations default to it. Called only by
// the `apilens env use` command (docs/05-cli.md section 3: "use writes
// .apilens/.current-env (gitignored)").
func (a *App) PersistCurrentEnv() error {
	name := string(a.Env.CurrentName())
	path := a.currentEnvFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewConfigError("creating .apilens directory", err)
	}
	if err := os.WriteFile(path, []byte(name), 0o644); err != nil {
		return domain.NewConfigError("writing .apilens/.current-env", err)
	}
	return nil
}

// RunFilter narrows which tests RunSuite executes. An empty RunFilter runs
// every test under .apilens/tests. See docs/04-interfaces.md section 14 and
// docs/05-cli.md sections on `run`/`test`.
type RunFilter struct {
	// Ref is a file path, url path substring, or test name — matches
	// docs/05-cli.md `apilens test <ref>` semantics. Empty Ref means "run
	// everything" (`apilens run`).
	Ref string
	// Method restricts matching when Ref is ambiguous across methods.
	Method string
	// Tag restricts to tests carrying this tag.
	Tag string
	// Sequential/Parallel/FailFast override config.yaml testing.* values.
	Sequential bool
	Parallel   bool
	FailFast   bool
	// ReporterFormat selects "terminal" or "json"; empty uses config default.
	ReporterFormat string
}

// RunSuite loads tests from .apilens/tests, applies filter, and executes
// them through testrunner. It implements docs/01-architecture.md's
// `RunSuite` / `RunTest` use cases (they share this one code path per
// docs/11-risks-and-gaps.md G2: "Run all tests whose request path matches").
func (a *App) RunSuite(ctx context.Context, filter RunFilter, out reporter.Reporter) (domain.Report, error) {
	testsDir := filepath.Join(a.ProjectDir, ".apilens", "tests")
	loader := testdef.NewLoader()
	tests, err := loader.LoadAll(testsDir)
	if err != nil {
		return domain.Report{}, err
	}

	tests = filterTests(tests, filter)
	if len(tests) == 0 {
		return domain.Report{}, domain.NewConfigError(
			"no tests matched — write a YAML test under .apilens/tests or run apilens generate", nil)
	}

	httpRunner := runner.New(runner.WithMaxResponseSize(a.Config.MaxResponseSizeBytes()))
	assertEngine := assertions.New()

	tr := testrunner.New(httpRunner, a.Env, assertEngine, out)

	opts := testrunner.Options{
		Parallel: a.Config.Testing.Parallel,
		Workers:  a.Config.Testing.Workers,
		Retries:  a.Config.Testing.Retries,
		Timeout:  a.Config.Testing.Timeout,
		FailFast: filter.FailFast,
	}
	if filter.Sequential {
		opts.Parallel = false
	}
	if filter.Parallel {
		opts.Parallel = true
	}

	return tr.Run(ctx, tests, opts)
}

// filterTests applies RunFilter to a loaded test set, implementing
// docs/05-cli.md `apilens test <ref>` and `--filter` matching rules.
func filterTests(tests []domain.TestCase, filter RunFilter) []domain.TestCase {
	if filter.Ref == "" && filter.Method == "" && filter.Tag == "" {
		return tests
	}
	var out []domain.TestCase
	for _, tc := range tests {
		if filter.Method != "" && string(tc.Request.Method) != domain.NormalizeMethod(filter.Method).String() {
			continue
		}
		if filter.Tag != "" && !tc.HasTag(filter.Tag) {
			continue
		}
		if filter.Ref != "" && !matchesRef(tc, filter.Ref) {
			continue
		}
		out = append(out, tc)
	}
	return out
}

// matchesRef matches a test file path exactly, or a name/URL substring —
// covers file path, URL path, and test name forms from docs/05-cli.md
// `apilens test <ref>`.
func matchesRef(tc domain.TestCase, ref string) bool {
	if tc.File == ref {
		return true
	}
	if tc.Name == ref {
		return true
	}
	return strings.Contains(tc.Request.URL, ref) || strings.Contains(tc.File, ref) || strings.Contains(tc.Name, ref)
}
