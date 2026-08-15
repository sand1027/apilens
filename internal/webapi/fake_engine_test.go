package webapi

import (
	"context"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

// fakeEngine implements apilens.Engine with scriptable return values, so
// webapi handler tests exercise only the HTTP translation layer — not a
// real Engine/App/runner stack. Each field is a function the test can set;
// zero-value fields return sane defaults.
type fakeEngine struct {
	initFn             func(ctx context.Context, path string, force bool) (apilens.InitResult, error)
	discoverFn         func(ctx context.Context, opts apilens.DiscoverOptions) (*apilens.DiscoverResult, error)
	listFn             func(filter apilens.EndpointFilter) []apilens.Endpoint
	inspectFn          func(ctx context.Context, ref apilens.InspectRef) (*apilens.Inspection, error)
	runFn              func(ctx context.Context, filter apilens.RunFilter) (*apilens.Report, error)
	watchFn            func(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error)
	replayFn           func(ctx context.Context, id int, overrides apilens.ReplayOverrides) (*domain.Exchange, error)
	generateFn         func(id int, opts apilens.GenerateOptions) (*apilens.GeneratedTest, error)
	historyFn          func(limit int) ([]domain.Exchange, error)
	historyGetFn       func(displayID int) (*domain.Exchange, bool, error)
	useEnvFn           func(name string) error
	environmentsFn     func() []domain.Environment
	currentEnvFn       func() domain.Environment
	specExportFn       func(opts apilens.SpecExportOptions) (string, error)
	contractTestFn     func(ctx context.Context, opts apilens.ContractTestOptions) (*apilens.Report, error)
	graphFn            func(opts apilens.GraphOptions) apilens.Graph
	mockFn             func(ctx context.Context, opts apilens.MockOptions) (*apilens.MockSession, error)
	pageMapFn          func() []apilens.PageMapping
	runLoadFn          func(ctx context.Context, filter apilens.RunFilter, opts apilens.LoadOptions) (*apilens.LoadReport, error)
	recordFn           func(opts apilens.RecordOptions) (apilens.RecordResult, error)
	generateFromSpecFn func(opts apilens.GenerateFromSpecOptions) (apilens.GenerateFromSpecResult, error)
}

func (f *fakeEngine) Init(ctx context.Context, path string, force bool) (apilens.InitResult, error) {
	if f.initFn != nil {
		return f.initFn(ctx, path, force)
	}
	return apilens.InitResult{}, nil
}

func (f *fakeEngine) Discover(ctx context.Context, opts apilens.DiscoverOptions) (*apilens.DiscoverResult, error) {
	if f.discoverFn != nil {
		return f.discoverFn(ctx, opts)
	}
	return &apilens.DiscoverResult{}, nil
}

func (f *fakeEngine) List(filter apilens.EndpointFilter) []apilens.Endpoint {
	if f.listFn != nil {
		return f.listFn(filter)
	}
	return nil
}

func (f *fakeEngine) Inspect(ctx context.Context, ref apilens.InspectRef) (*apilens.Inspection, error) {
	if f.inspectFn != nil {
		return f.inspectFn(ctx, ref)
	}
	return &apilens.Inspection{}, nil
}

func (f *fakeEngine) Run(ctx context.Context, filter apilens.RunFilter) (*apilens.Report, error) {
	if f.runFn != nil {
		return f.runFn(ctx, filter)
	}
	return &apilens.Report{}, nil
}

func (f *fakeEngine) Watch(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error) {
	if f.watchFn != nil {
		return f.watchFn(ctx, opts)
	}
	return &apilens.WatchSession{}, nil
}

func (f *fakeEngine) Replay(ctx context.Context, id int, overrides apilens.ReplayOverrides) (*domain.Exchange, error) {
	if f.replayFn != nil {
		return f.replayFn(ctx, id, overrides)
	}
	return &domain.Exchange{}, nil
}

func (f *fakeEngine) Generate(id int, opts apilens.GenerateOptions) (*apilens.GeneratedTest, error) {
	if f.generateFn != nil {
		return f.generateFn(id, opts)
	}
	return &apilens.GeneratedTest{}, nil
}

func (f *fakeEngine) History(limit int) ([]domain.Exchange, error) {
	if f.historyFn != nil {
		return f.historyFn(limit)
	}
	return nil, nil
}

func (f *fakeEngine) HistoryGet(displayID int) (*domain.Exchange, bool, error) {
	if f.historyGetFn != nil {
		return f.historyGetFn(displayID)
	}
	return nil, false, nil
}

func (f *fakeEngine) UseEnv(name string) error {
	if f.useEnvFn != nil {
		return f.useEnvFn(name)
	}
	return nil
}

func (f *fakeEngine) Environments() []domain.Environment {
	if f.environmentsFn != nil {
		return f.environmentsFn()
	}
	return nil
}

func (f *fakeEngine) CurrentEnv() domain.Environment {
	if f.currentEnvFn != nil {
		return f.currentEnvFn()
	}
	return domain.Environment{}
}

func (f *fakeEngine) PersistCurrentEnv() error { return nil }

func (f *fakeEngine) SpecExport(opts apilens.SpecExportOptions) (string, error) {
	if f.specExportFn != nil {
		return f.specExportFn(opts)
	}
	return "", nil
}

func (f *fakeEngine) ContractTest(ctx context.Context, opts apilens.ContractTestOptions) (*apilens.Report, error) {
	if f.contractTestFn != nil {
		return f.contractTestFn(ctx, opts)
	}
	return &apilens.Report{}, nil
}

func (f *fakeEngine) Graph(opts apilens.GraphOptions) apilens.Graph {
	if f.graphFn != nil {
		return f.graphFn(opts)
	}
	return apilens.Graph{}
}

func (f *fakeEngine) Mock(ctx context.Context, opts apilens.MockOptions) (*apilens.MockSession, error) {
	if f.mockFn != nil {
		return f.mockFn(ctx, opts)
	}
	return &apilens.MockSession{}, nil
}

func (f *fakeEngine) Record(opts apilens.RecordOptions) (apilens.RecordResult, error) {
	if f.recordFn != nil {
		return f.recordFn(opts)
	}
	return apilens.RecordResult{}, nil
}

func (f *fakeEngine) GenerateFromSpec(opts apilens.GenerateFromSpecOptions) (apilens.GenerateFromSpecResult, error) {
	if f.generateFromSpecFn != nil {
		return f.generateFromSpecFn(opts)
	}
	return apilens.GenerateFromSpecResult{}, nil
}

func (f *fakeEngine) PageMap() []apilens.PageMapping {
	if f.pageMapFn != nil {
		return f.pageMapFn()
	}
	return nil
}

func (f *fakeEngine) RunLoad(ctx context.Context, filter apilens.RunFilter, opts apilens.LoadOptions) (*apilens.LoadReport, error) {
	if f.runLoadFn != nil {
		return f.runLoadFn(ctx, filter, opts)
	}
	return &apilens.LoadReport{}, nil
}
