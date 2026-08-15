// Package apilens is the only public Go surface of ApiLens (ADR-010). The
// CLI, and any future surface (web dashboard, CI integration), depend on
// this Engine interface — not on internal/* directly
// (docs/02-packages.md section 4, docs/04-interfaces.md section 14).
package apilens

import (
	"context"
	"time"

	"github.com/sandeepv/apilens/internal/app"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
	"github.com/sandeepv/apilens/internal/loadtest"
	"github.com/sandeepv/apilens/internal/project"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/replay"
	"github.com/sandeepv/apilens/internal/reporter"
)

// RunFilter narrows which tests Run executes. Mirrors internal/app.RunFilter
// so surfaces never need to import internal/app.
type RunFilter = app.RunFilter

// Report is re-exported so callers don't need internal/domain either.
type Report = domain.Report

// Endpoint, Inspection etc. are declared for interface completeness ahead
// of v2 (docs/04-interfaces.md section 14) even though Discover/Inspect
// return ErrNotImplemented until then.
type Endpoint = domain.Endpoint

// Inspection is the result of Engine.Inspect: the registry spec, and,
// only when InspectRef.Live was set, the live probe exchange (ADR-018).
type Inspection struct {
	Endpoint Endpoint
	Live     *domain.Exchange
}

// DiscoverError is a non-fatal per-provider failure from Discover
// (docs/11-risks-and-gaps.md R1: one bad OpenAPI file does not abort the
// whole run).
type DiscoverError struct {
	Provider string
	Message  string
}

// DiscoverResult is Engine.Discover's return shape: the merged endpoints
// plus any non-fatal provider errors.
type DiscoverResult struct {
	Endpoints []Endpoint
	Errors    []DiscoverError
}

// InitResult reports what Engine.Init created vs. left untouched.
type InitResult struct {
	Created  []string
	Skipped  []string
	Detected string
	Notes    []string
}

// DiscoverOptions configures Engine.Discover (v2).
type DiscoverOptions struct {
	Enabled []string
	Paths   []string
}

// InspectRef identifies what to inspect (v2): a path or an endpoint ID.
type InspectRef struct {
	Ref    string
	Method string
	Live   bool
	Last   bool
}

// WatchOptions configures Engine.Watch (docs/05-cli.md "apilens watch",
// docs/08-proxy.md). MITM is reserved for a later phase per ADR-020 and is
// not read yet.
type WatchOptions struct {
	Bind        string
	Port        int
	AllowRemote bool
	Upstream    string
	PathPrefix  string
	Host        string
	All         bool
	MITM        bool
}

// WatchSession is returned by Engine.Watch: the address actually bound,
// where the session JSONL file lives (for cross-terminal replay/generate —
// docs/08-proxy.md section 3), and the channel of captured exchanges.
// Extending docs/04-interfaces.md section 14's channel-only sketch was
// necessary so the CLI can print the watch banner before any traffic
// arrives.
type WatchSession struct {
	Addr        string
	SessionFile string
	Events      <-chan domain.Exchange
}

// Engine is the facade every surface depends on
// (docs/04-interfaces.md section 14). v1 implements Init, Run, and UseEnv.
// The remaining methods return domain.ErrNotImplemented until their
// product version ships (docs/10-plan.md section 10: "Returning
// ErrNotImplemented from unused Engine methods is fine").
type Engine interface {
	// Init creates the .apilens/ project layout at path. force controls
	// whether an existing config.yaml is overwritten (docs/05-cli.md
	// "apilens init": "Refuses to overwrite config.yaml unless --force").
	// InitResult is returned so the CLI can print exactly what changed.
	Init(ctx context.Context, path string, force bool) (InitResult, error)
	// Discover runs enabled discovery providers, merges their endpoints
	// into the registry, and persists .apilens/api/registry.yaml
	// (docs/07-discovery.md section 1). Non-fatal per-provider failures
	// are returned in DiscoverResult.Errors rather than aborting the run.
	Discover(ctx context.Context, opts DiscoverOptions) (*DiscoverResult, error)
	// List reads the registry as it stands (no implicit re-discover —
	// docs/05-cli.md "apilens list").
	List(filter EndpointFilter) []Endpoint
	// Inspect resolves ref (a path or endpoint ID) against the registry
	// spec by default; InspectRef.Live triggers a probe through the same
	// shared runner every other live call uses (ADR-018).
	Inspect(ctx context.Context, ref InspectRef) (*Inspection, error)
	Run(ctx context.Context, filter RunFilter) (*Report, error)
	// Watch starts the local forward proxy and blocks internally until ctx
	// is canceled (the returned WatchSession's Events channel closes at
	// that point) — see docs/08-proxy.md section 3, "watch blocks the
	// terminal". The bind policy (docs/09-security.md section 5) is
	// enforced before this returns, so a bad --bind or a port already in
	// use surfaces as an error immediately.
	Watch(ctx context.Context, opts WatchOptions) (*WatchSession, error)
	Replay(ctx context.Context, id int, overrides ReplayOverrides) (*domain.Exchange, error)
	Generate(id int, opts GenerateOptions) (*GeneratedTest, error)
	// History lists captured exchanges: in-memory if `watch` is running in
	// this process, otherwise from the session JSONL file
	// (docs/08-proxy.md section 3). An unreadable session file is an error
	// rather than an empty list.
	History(limit int) ([]domain.Exchange, error)
	// HistoryGet looks up one exchange by its session-scoped display ID
	// (ADR-013: "#42 is not portable across sessions").
	HistoryGet(displayID int) (*domain.Exchange, bool, error)
	UseEnv(name string) error

	// Environments and CurrentEnv back `apilens env list|show`. Not in the
	// original docs/04-interfaces.md sketch, but required so the CLI never
	// reads internal/environment directly (docs/01-architecture.md section 7,
	// "shared engine rule"). Interface shapes may tighten during
	// implementation per docs/04-interfaces.md's own note.
	Environments() []domain.Environment
	CurrentEnv() domain.Environment
	// PersistCurrentEnv writes the current selection to
	// .apilens/.current-env. Only `apilens env use` calls this.
	PersistCurrentEnv() error

	// SpecExport writes an OpenAPI 3 document built from the registry plus
	// captured exchange examples (plan.md v7: "OpenAPI generation from
	// registry + captures"). Returns the path actually written.
	SpecExport(opts SpecExportOptions) (string, error)
	// ContractTest validates captured (and, if opts.Live, live-probed)
	// responses against the response schemas declared by the OpenAPI
	// document at opts.SpecPath (plan.md v7: "Contract testing against a
	// stored spec").
	ContractTest(ctx context.Context, opts ContractTestOptions) (*Report, error)

	// Graph builds an inferred API dependency graph from captured traffic
	// (plan.md v8: "API dependency graphs").
	Graph(opts GraphOptions) Graph
	// PageMap groups captured traffic by the Referer header of the page
	// that triggered it (plan.md v8: "Page-to-API mapping"). This is a
	// best-effort signal, not ground truth — see internal/pagemap's
	// package doc / docs/11-risks-and-gaps.md G19.
	PageMap() []PageMapping

	// RunLoad repeats the filtered test set across concurrent workers for
	// a bounded duration or iteration count, reporting latency
	// percentiles and error rate rather than pass/fail (plan.md v9:
	// "Load / soak mode on the same runner"). DSL v2 chained tests are
	// rejected — see internal/loadtest's package doc.
	RunLoad(ctx context.Context, filter RunFilter, opts LoadOptions) (*LoadReport, error)

	// Record turns the current history into a chained DSL v2 suite on
	// disk (plan.md v9: "Test recording sessions").
	Record(opts RecordOptions) (RecordResult, error)

	// GenerateFromSpec writes one YAML v1 test per OpenAPI operation that
	// declares a usable example (plan.md v9: "Automatic test generation
	// from OpenAPI examples").
	GenerateFromSpec(opts GenerateFromSpecOptions) (GenerateFromSpecResult, error)
	// Mock starts a local mock server serving canned responses built from
	// the registry and captured examples (plan.md v8: "API mocking
	// (apilens mock) using registry + captured examples"). Mirrors
	// Watch's shape: it enforces the same loopback bind policy and blocks
	// internally until ctx is canceled.
	Mock(ctx context.Context, opts MockOptions) (*MockSession, error)
}

// EndpointFilter mirrors registry.Filter (docs/04-interfaces.md section 3).
// Defined here rather than imported so callers of pkg/apilens never need
// internal/registry.
type EndpointFilter struct {
	Method string
	Path   string
	Tag    string
	Source string
}

// ReplayOverrides mirrors replay.Overrides (v4).
type ReplayOverrides struct {
	Method  *string
	URL     *string
	Headers map[string]string
	Unset   []string
	Query   map[string]string
	Body    []byte
}

// GenerateOptions mirrors generate.Options (v4).
type GenerateOptions struct {
	Out   string
	Force bool
}

// GeneratedTest mirrors generate.Generated (v4).
type GeneratedTest struct {
	Path    string
	Content []byte
	Test    domain.TestCase
}

// SpecExportOptions configures Engine.SpecExport (v7).
type SpecExportOptions struct {
	Out     string
	Force   bool
	Title   string
	Version string
}

// ContractTestOptions configures Engine.ContractTest (v7).
type ContractTestOptions struct {
	SpecPath string
	Live     bool
}

// GraphOptions configures Engine.Graph (v8).
type GraphOptions struct {
	WindowMS int
}

// GraphNode/GraphEdge/Graph mirror internal/graph's types (v8), re-exported
// so callers don't need internal/graph either.
type GraphNode struct {
	Method string
	Path   string
}

type GraphEdge struct {
	From   GraphNode
	To     GraphNode
	Weight int
}

type Graph struct {
	Nodes []GraphNode
	Edges []GraphEdge
}

// MockOptions configures Engine.Mock (v8).
type MockOptions struct {
	Bind        string
	Port        int
	AllowRemote bool
}

// MockSession is returned by Engine.Mock: the address actually bound.
type MockSession struct {
	Addr string
}

// LoadOptions configures Engine.RunLoad (v9). Mirrors internal/loadtest's
// own Options — re-exported so callers don't need internal/loadtest.
type LoadOptions struct {
	Duration   time.Duration
	Iterations int
	Workers    int
	Timeout    time.Duration
}

// Percentiles/TestStats/LoadReport mirror internal/loadtest's Report
// shape (v9) — a repeated-execution report, deliberately distinct from
// Report (which is pass/fail-oriented).
type Percentiles = loadtest.Percentiles
type TestStats = loadtest.TestStats
type LoadReport = loadtest.Report

// RecordOptions configures Engine.Record (v9).
type RecordOptions struct {
	Limit int
	Out   string
	Force bool
}

// RecordResult mirrors internal/app.RecordResult (v9).
type RecordResult struct {
	Files []string
	Steps int
}

// GenerateFromSpecOptions configures Engine.GenerateFromSpec (v9).
type GenerateFromSpecOptions struct {
	SpecPath string
	Out      string
	Force    bool
}

// GenerateFromSpecResult mirrors internal/app.GenerateFromSpecResult (v9).
type GenerateFromSpecResult struct {
	Files    []string
	Skipped  int
	Compiled int
}

// APICall/PageMapping mirror internal/pagemap's types (v8).
type APICall struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Count  int    `json:"count"`
}

type PageMapping struct {
	Page  string    `json:"page"`
	Calls []APICall `json:"calls"`
}

// engine is the concrete Engine implementation, composing internal/app
// (docs/02-packages.md section 4).
type engine struct {
	app        *app.App
	projectDir string
	reporters  *reporter.Registry
}

// Options configures New.
type Options struct {
	ProjectDir string
	ConfigPath string
	EnvDir     string
}

// New builds an Engine rooted at opts.ProjectDir. It loads config and
// environments immediately so UseEnv/Run can be called right away; callers
// that only want Init should still call New first (Init works even if
// .apilens/ doesn't exist yet, since config.Load and environment.LoadDir
// both tolerate a missing directory).
func New(opts Options) (Engine, error) {
	a, err := app.New(app.Options{
		ProjectDir: opts.ProjectDir,
		ConfigPath: opts.ConfigPath,
		EnvDir:     opts.EnvDir,
	})
	if err != nil {
		return nil, err
	}
	return &engine{app: a, projectDir: opts.ProjectDir}, nil
}

func (e *engine) Init(ctx context.Context, path string, force bool) (InitResult, error) {
	res, err := project.Init(path, force)
	if err != nil {
		return InitResult{}, err
	}
	return InitResult{
		Created:  res.Created,
		Skipped:  res.Skipped,
		Detected: res.Detected,
		Notes:    res.Notes,
	}, nil
}

func (e *engine) Discover(ctx context.Context, opts DiscoverOptions) (*DiscoverResult, error) {
	res, err := e.app.Discover(ctx, app.DiscoverOptions{Enabled: opts.Enabled, Paths: opts.Paths})
	if err != nil {
		return nil, err
	}
	out := &DiscoverResult{Endpoints: res.Endpoints}
	for _, e := range res.Errors {
		out.Errors = append(out.Errors, DiscoverError{Provider: e.Provider, Message: e.Err.Error()})
	}
	return out, nil
}

func (e *engine) List(filter EndpointFilter) []Endpoint {
	return e.app.List(registry.Filter{
		Method: filter.Method,
		Path:   filter.Path,
		Tag:    filter.Tag,
		Source: filter.Source,
	})
}

func (e *engine) Inspect(ctx context.Context, ref InspectRef) (*Inspection, error) {
	insp, err := e.app.Inspect(ctx, app.InspectRef{Ref: ref.Ref, Method: ref.Method, Live: ref.Live})
	if err != nil {
		return nil, err
	}
	return &Inspection{Endpoint: insp.Endpoint, Live: insp.Live}, nil
}

// Run executes the test suite (or filtered subset) and returns the report.
// It selects the reporter implied by filter.ReporterFormat, defaulting to
// the configured testing.reporter.
func (e *engine) Run(ctx context.Context, filter RunFilter) (*Report, error) {
	format := filter.ReporterFormat
	if format == "" {
		format = e.app.Config.Testing.Reporter
	}
	rep, ok := e.app.Host.Reporters.Get(format)
	if !ok {
		return nil, domain.NewConfigError("unknown reporter format \""+format+"\"", nil)
	}
	if filter.Quiet {
		// Only the terminal reporter has per-test noise to suppress;
		// json/junit already buffer and emit once regardless
		// (docs/05-cli.md --quiet: "Errors only").
		if quieter, ok := rep.(interface{ SetQuiet(bool) }); ok {
			quieter.SetQuiet(true)
		}
	}
	report, err := e.app.RunSuite(ctx, filter, rep)
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (e *engine) Watch(ctx context.Context, opts WatchOptions) (*WatchSession, error) {
	handle, err := e.app.Watch(ctx, app.WatchOptions{
		Bind:        opts.Bind,
		Port:        opts.Port,
		AllowRemote: opts.AllowRemote,
		Upstream:    opts.Upstream,
		PathPrefix:  opts.PathPrefix,
		Host:        opts.Host,
		All:         opts.All,
	})
	if err != nil {
		return nil, err
	}
	return &WatchSession{
		Addr:        handle.Addr,
		SessionFile: handle.SessionFile.Path(),
		Events:      handle.Events,
	}, nil
}

func (e *engine) Replay(ctx context.Context, id int, overrides ReplayOverrides) (*domain.Exchange, error) {
	ex, err := e.app.Replay(ctx, id, replay.Overrides{
		Method:  overrides.Method,
		URL:     overrides.URL,
		Headers: overrides.Headers,
		Unset:   overrides.Unset,
		Query:   overrides.Query,
		Body:    overrides.Body,
	})
	if err != nil {
		return nil, err
	}
	return &ex, nil
}

func (e *engine) Generate(id int, opts GenerateOptions) (*GeneratedTest, error) {
	result, err := e.app.Generate(id, generate.Options{Out: opts.Out, Force: opts.Force})
	if err != nil {
		return nil, err
	}
	return &GeneratedTest{Path: result.Path, Content: result.Content, Test: result.Test}, nil
}

func (e *engine) History(limit int) ([]domain.Exchange, error) {
	return e.app.HistoryList(limit)
}

func (e *engine) HistoryGet(displayID int) (*domain.Exchange, bool, error) {
	ex, ok, err := e.app.HistoryGet(displayID)
	if err != nil || !ok {
		return nil, ok, err
	}
	return &ex, true, nil
}

func (e *engine) UseEnv(name string) error {
	return e.app.UseEnv(name)
}

func (e *engine) Environments() []domain.Environment {
	return e.app.Env.List()
}

func (e *engine) CurrentEnv() domain.Environment {
	return e.app.Env.Current()
}

func (e *engine) PersistCurrentEnv() error {
	return e.app.PersistCurrentEnv()
}

func (e *engine) SpecExport(opts SpecExportOptions) (string, error) {
	return e.app.SpecExport(app.SpecExportOptions{
		Out:     opts.Out,
		Force:   opts.Force,
		Title:   opts.Title,
		Version: opts.Version,
	})
}

func (e *engine) ContractTest(ctx context.Context, opts ContractTestOptions) (*Report, error) {
	report, err := e.app.ContractTest(ctx, app.ContractTestOptions{SpecPath: opts.SpecPath, Live: opts.Live})
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (e *engine) Graph(opts GraphOptions) Graph {
	g := e.app.Graph(app.GraphOptions{WindowMS: opts.WindowMS})
	out := Graph{}
	for _, n := range g.Nodes {
		out.Nodes = append(out.Nodes, GraphNode{Method: string(n.Method), Path: n.Path})
	}
	for _, ed := range g.Edges {
		out.Edges = append(out.Edges, GraphEdge{
			From:   GraphNode{Method: string(ed.From.Method), Path: ed.From.Path},
			To:     GraphNode{Method: string(ed.To.Method), Path: ed.To.Path},
			Weight: ed.Weight,
		})
	}
	return out
}

func (e *engine) Mock(ctx context.Context, opts MockOptions) (*MockSession, error) {
	handle, err := e.app.Mock(ctx, app.MockOptions{
		Bind:        opts.Bind,
		Port:        opts.Port,
		AllowRemote: opts.AllowRemote,
	})
	if err != nil {
		return nil, err
	}
	return &MockSession{Addr: handle.Addr}, nil
}

func (e *engine) RunLoad(ctx context.Context, filter RunFilter, opts LoadOptions) (*LoadReport, error) {
	report, err := e.app.RunLoad(ctx, filter, app.LoadOptions{
		Duration:   opts.Duration,
		Iterations: opts.Iterations,
		Workers:    opts.Workers,
		Timeout:    opts.Timeout,
	})
	if err != nil {
		return nil, err
	}
	return &report, nil
}

func (e *engine) Record(opts RecordOptions) (RecordResult, error) {
	res, err := e.app.Record(app.RecordOptions{Limit: opts.Limit, Out: opts.Out, Force: opts.Force})
	if err != nil {
		return RecordResult{}, err
	}
	return RecordResult{Files: res.Files, Steps: res.Steps}, nil
}

func (e *engine) GenerateFromSpec(opts GenerateFromSpecOptions) (GenerateFromSpecResult, error) {
	res, err := e.app.GenerateFromSpec(app.GenerateFromSpecOptions{SpecPath: opts.SpecPath, Out: opts.Out, Force: opts.Force})
	if err != nil {
		return GenerateFromSpecResult{}, err
	}
	return GenerateFromSpecResult{Files: res.Files, Skipped: res.Skipped, Compiled: res.Compiled}, nil
}

func (e *engine) PageMap() []PageMapping {
	mappings := e.app.PageMap()
	out := make([]PageMapping, 0, len(mappings))
	for _, m := range mappings {
		pm := PageMapping{Page: m.Page}
		for _, c := range m.Calls {
			pm.Calls = append(pm.Calls, APICall{Method: string(c.Method), Path: c.Path, Count: c.Count})
		}
		out = append(out, pm)
	}
	return out
}
