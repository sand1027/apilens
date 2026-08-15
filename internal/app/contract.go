package app

import (
	"context"
	"path/filepath"

	"github.com/sandeepv/apilens/internal/contract"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/runner"
	"github.com/sandeepv/apilens/internal/specexport"
)

// SpecExportOptions configures SpecExport (docs/05-cli.md-style flags:
// --out/--force plus doc metadata).
type SpecExportOptions struct {
	Out     string // output path; empty defaults to .apilens/api/export.openapi.yaml
	Force   bool
	Title   string
	Version string
}

// SpecExport builds an OpenAPI 3 document from the current registry plus
// the most recent captured exchange per endpoint (from history), and
// writes it to disk. Implements plan.md v7: "OpenAPI generation from
// registry + captures" / "apilens spec export writes an OpenAPI file the
// team can review".
func (a *App) SpecExport(opts SpecExportOptions) (string, error) {
	endpoints := a.Registry.List(registry.Filter{})
	if len(endpoints) == 0 {
		return "", domain.NewNotFoundError(
			"registry is empty — run apilens discover (or apilens watch) first")
	}

	examples := a.latestExchangePerEndpoint(endpoints)

	doc := specexport.Build(endpoints, specexport.Options{
		Title:    opts.Title,
		Version:  opts.Version,
		BaseURL:  a.baseURLHint(),
		Examples: examples,
	})

	path := opts.Out
	if path == "" {
		path = defaultExportPath(a.ProjectDir)
	}
	if err := specexport.WriteFile(doc, path, opts.Force); err != nil {
		return "", err
	}
	return path, nil
}

// ContractTestOptions configures ContractTest.
type ContractTestOptions struct {
	SpecPath string // required: path to a stored OpenAPI document
	Live     bool   // probe live for endpoints with no captured example
}

// ContractTest loads the OpenAPI document at opts.SpecPath and validates
// captured (and, if opts.Live, live-probed) responses against its
// response schemas, per endpoint. Implements plan.md v7: "Contract
// testing against a stored spec".
func (a *App) ContractTest(ctx context.Context, opts ContractTestOptions) (domain.Report, error) {
	if opts.SpecPath == "" {
		return domain.Report{}, domain.NewConfigError("contract test requires --spec <path>", nil)
	}

	doc, err := contract.Load(opts.SpecPath)
	if err != nil {
		return domain.Report{}, err
	}

	endpoints := a.Registry.List(registry.Filter{})
	examples := a.latestExchangePerEndpoint(endpoints)

	var probe contract.Prober
	if opts.Live {
		httpRunner := runner.New(runner.WithMaxResponseSize(a.Config.MaxResponseSizeBytes()))
		probe = func(ctx context.Context, method, path string) (domain.Exchange, error) {
			url, err := a.Env.Interpolate("{{base_url}}" + path)
			if err != nil {
				return domain.Exchange{}, err
			}
			return httpRunner.Do(ctx, domain.HTTPRequest{Method: domain.Method(method), URL: url})
		}
	}

	return contract.Run(ctx, doc, probe, contract.Options{Examples: examples})
}

// latestExchangePerEndpoint scans history (most recent first) and returns
// the newest captured exchange for each endpoint identity, for use as a
// specexport/contract "example" without needing a live probe.
func (a *App) latestExchangePerEndpoint(endpoints []domain.Endpoint) map[domain.EndpointID]domain.Exchange {
	examples := make(map[domain.EndpointID]domain.Exchange)
	all := a.HistoryList(0)
	// HistoryList returns oldest-first; walk in reverse so the first hit
	// per endpoint ID is the most recent capture.
	for i := len(all) - 1; i >= 0; i-- {
		ex := all[i]
		path := requestPath(ex.Request.URL)
		id := domain.NewEndpointID(ex.Request.Method, path)
		if _, ok := examples[id]; ok {
			continue
		}
		examples[id] = ex
	}
	return examples
}

// baseURLHint returns the current environment's base_url (if resolvable)
// to seed the exported document's servers[0].url — a hint only, per
// docs/07-discovery.md section 3.
func (a *App) baseURLHint() string {
	url, err := a.Env.Interpolate("{{base_url}}")
	if err != nil {
		return ""
	}
	return url
}

func defaultExportPath(projectDir string) string {
	return filepath.Join(projectDir, ".apilens", "api", "export.openapi.yaml")
}
