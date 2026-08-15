package app

import (
	"context"
	"os"

	"github.com/sandeepv/apilens/internal/discovery"
	"github.com/sandeepv/apilens/internal/discovery/providers/echo"
	"github.com/sandeepv/apilens/internal/discovery/providers/express"
	"github.com/sandeepv/apilens/internal/discovery/providers/fastify"
	"github.com/sandeepv/apilens/internal/discovery/providers/fiber"
	"github.com/sandeepv/apilens/internal/discovery/providers/gin"
	gqlprovider "github.com/sandeepv/apilens/internal/discovery/providers/graphql"
	"github.com/sandeepv/apilens/internal/discovery/providers/nestjs"
	"github.com/sandeepv/apilens/internal/discovery/providers/openapi"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/runner"
)

// DiscoverOptions mirrors docs/04-interfaces.md section 2's Options,
// exposed at the app layer so the CLI's --source/--path flags have
// somewhere to land.
type DiscoverOptions struct {
	Enabled []string // --source
	Paths   []string // --path
}

// DiscoverResult carries the merged endpoints plus any non-fatal
// per-provider errors, so the CLI can print "Found: N APIs" and still warn
// about a broken OpenAPI file without aborting (docs/11-risks-and-gaps.md
// R1).
type DiscoverResult struct {
	Endpoints []domain.Endpoint
	Errors    []discovery.ProviderError
}

// buildOrchestrator wires the built-in providers per current config
// (docs/03-plugins.md section 9: "Disabled plugins are registered but
// skipped by the orchestrator"). Table-driven rather than one
// if-statement per provider (v8 added 5 framework providers on top of
// v2's openapi/express, and this shape stays flat as more are added —
// v8's follow-up providers per plan.md, e.g. Spring/Django/ASP.NET, only
// need one more line each here plus a DiscoveryConfig field).
func (a *App) buildOrchestrator() *discovery.Orchestrator {
	entries := []struct {
		provider discovery.Provider
		enabled  bool
	}{
		{openapi.New(a.Config.Discovery.OpenAPI.Paths), a.Config.Discovery.OpenAPI.Enabled},
		{gqlprovider.New(a.Config.Discovery.GraphQL.Paths), a.Config.Discovery.GraphQL.Enabled},
		{express.New(), a.Config.Discovery.Express.Enabled},
		{fastify.New(), a.Config.Discovery.Fastify.Enabled},
		{nestjs.New(), a.Config.Discovery.NestJS.Enabled},
		{gin.New(), a.Config.Discovery.Gin.Enabled},
		{fiber.New(), a.Config.Discovery.Fiber.Enabled},
		{echo.New(), a.Config.Discovery.Echo.Enabled},
	}

	providers := make([]discovery.Provider, 0, len(entries))
	var disabled []string
	for _, e := range entries {
		providers = append(providers, e.provider)
		if !e.enabled {
			disabled = append(disabled, e.provider.Name())
		}
	}
	return discovery.New(providers, disabled)
}

// Discover runs the orchestrator against the project root, replaces the
// registry, and persists it to .apilens/api/registry.yaml
// (docs/07-discovery.md section 1, ADR-017).
func (a *App) Discover(ctx context.Context, opts DiscoverOptions) (DiscoverResult, error) {
	orch := a.buildOrchestrator()
	rootFS := os.DirFS(a.ProjectDir)

	endpoints, errs, err := orch.Discover(ctx, rootFS, discovery.Options{
		Enabled: opts.Enabled,
		Paths:   opts.Paths,
	})
	if err != nil {
		return DiscoverResult{}, err
	}

	// Discover-time tag/ignore filters (plan.md v8) are applied after the
	// orchestrator's own merge, so they see the final deduped endpoint
	// set rather than each provider's raw output.
	endpoints = discovery.ApplyFilters(endpoints, discovery.FilterOptions{
		Ignore: a.Config.Discovery.Ignore,
		Tag:    a.Config.Discovery.Tags,
	})

	if err := a.Registry.Replace(endpoints); err != nil {
		return DiscoverResult{}, domain.NewConfigError("updating registry", err)
	}
	if err := registry.SaveYAML(registryPath(a.ProjectDir), endpoints); err != nil {
		return DiscoverResult{}, err
	}

	return DiscoverResult{Endpoints: endpoints, Errors: errs}, nil
}

// List reads the in-memory (possibly disk-hydrated) registry. It never
// re-discovers implicitly (docs/05-cli.md "apilens list": "Does not
// rediscover implicitly").
func (a *App) List(filter registry.Filter) []domain.Endpoint {
	return a.Registry.List(filter)
}

// InspectRef identifies what to inspect: a path or endpoint ID, optionally
// disambiguated by method, and optionally backed by a live probe
// (docs/05-cli.md "apilens inspect <ref>", ADR-018: spec-first by default).
type InspectRef struct {
	Ref    string
	Method string
	Live   bool
}

// Inspection is the result of Inspect: the registry spec, and — only when
// Live was requested — the live HTTP exchange from probing it through the
// shared runner (docs/01-architecture.md section 5: every live call goes
// through the same runner).
type Inspection struct {
	Endpoint domain.Endpoint
	Live     *domain.Exchange
}

// Inspect resolves ref against the registry (spec-first per ADR-018) and,
// only if Live is set, performs a probe request through internal/runner —
// the same shared runner every other live call uses.
func (a *App) Inspect(ctx context.Context, ref InspectRef) (*Inspection, error) {
	ep, err := a.resolveEndpoint(ref.Ref, ref.Method)
	if err != nil {
		return nil, err
	}

	insp := &Inspection{Endpoint: ep}
	if !ref.Live {
		return insp, nil
	}

	httpRunner := runner.New(runner.WithMaxResponseSize(a.Config.MaxResponseSizeBytes()))
	url, err := a.Env.Interpolate("{{base_url}}" + ep.Path)
	if err != nil {
		return nil, err
	}
	ex, err := httpRunner.Do(ctx, domain.HTTPRequest{Method: ep.Method, URL: url})
	if err != nil {
		// A transport error during --live is still useful to show, not a
		// hard failure of the inspect command itself.
		insp.Live = &ex
		return insp, nil
	}
	insp.Live = &ex
	return insp, nil
}

// resolveEndpoint finds a registry entry matching ref (a path or an
// EndpointID), disambiguating by method when more than one shares the
// path (docs/05-cli.md: "--method Required when multiple methods share
// the path").
func (a *App) resolveEndpoint(ref, method string) (domain.Endpoint, error) {
	if ep, ok := a.Registry.GetByID(domain.EndpointID(ref)); ok {
		return ep, nil
	}

	matches := a.Registry.List(registry.Filter{Path: ref})
	// Filter.Path is a substring match; narrow to an exact normalized
	// path match first, since callers usually pass an exact path.
	var exact []domain.Endpoint
	for _, ep := range matches {
		if domain.NormalizePath(ep.Path) == domain.NormalizePath(ref) {
			exact = append(exact, ep)
		}
	}
	if len(exact) > 0 {
		matches = exact
	}

	if method != "" {
		var filtered []domain.Endpoint
		for _, ep := range matches {
			if string(ep.Method) == string(domain.NormalizeMethod(method)) {
				filtered = append(filtered, ep)
			}
		}
		matches = filtered
	}

	switch len(matches) {
	case 0:
		return domain.Endpoint{}, domain.NewNotFoundError(
			"no endpoint matched \"" + ref + "\" — run apilens discover first")
	case 1:
		return matches[0], nil
	default:
		return domain.Endpoint{}, domain.NewConfigError(
			"multiple methods share \""+ref+"\" — pass --method to disambiguate", nil)
	}
}
