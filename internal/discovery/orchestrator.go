package discovery

import (
	"context"
	"io/fs"
	"sort"

	"github.com/sandeepv/apilens/internal/domain"
)

// sourcePriority implements docs/07-discovery.md section 2:
// "Priority: openapi > framework AST > runtime watch." Lower number wins
// when picking the richest Spec / display path form. Unknown sources sort
// after every known one.
var sourcePriority = map[string]int{
	"openapi":  0,
	"graphql":  0,
	"express":  1,
	"fastify": 1,
	"nestjs":  1,
	"gin":     1,
	"fiber":   1,
	"echo":    1,
	"watch":   2,
}

func priorityOf(source string) int {
	if p, ok := sourcePriority[source]; ok {
		return p
	}
	return len(sourcePriority)
}

// Orchestrator implements docs/04-interfaces.md section 2's Orchestrator
// interface and the merge algorithm in docs/07-discovery.md section 7.
type Orchestrator struct {
	providers []Provider
	// disabled holds provider names turned off in config
	// (docs/03-plugins.md section 9: "Disabled plugins are registered but
	// skipped by the orchestrator").
	disabled map[string]bool
}

// New builds an Orchestrator over the given providers. disabled lists
// provider names disabled by config (e.g. discovery.express.enabled: false).
func New(providers []Provider, disabled []string) *Orchestrator {
	disabledSet := make(map[string]bool, len(disabled))
	for _, d := range disabled {
		disabledSet[d] = true
	}
	return &Orchestrator{providers: providers, disabled: disabledSet}
}

// Discover runs every eligible provider, then normalizes, groups, and
// merges their endpoints per docs/07-discovery.md section 7. Providers are
// run even if one fails; a single bad OpenAPI file does not abort the
// whole run unless --path pointed directly at it (docs/11-risks-and-gaps.md
// R1) — that policy is enforced by passing opts.Paths only to providers
// that were explicitly named via --source, which callers control.
func (o *Orchestrator) Discover(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, []ProviderError, error) {
	var all []domain.Endpoint
	var errs []ProviderError

	explicitPath := len(opts.Paths) > 0

	for _, p := range o.providers {
		name := p.Name()
		if o.disabled[name] {
			continue
		}
		if !enabled(name, opts.Enabled) {
			continue
		}

		if !explicitPath {
			ok, err := p.Detect(ctx, root)
			if err != nil {
				errs = append(errs, ProviderError{Provider: name, Err: err})
				continue
			}
			if !ok {
				continue
			}
		}

		found, err := p.Discover(ctx, root, opts)
		if err != nil {
			errs = append(errs, ProviderError{Provider: name, Err: err})
			continue
		}
		all = append(all, found...)
	}

	return mergeEndpoints(all), errs, nil
}

// ProviderError records a non-fatal failure from a single provider
// (docs/11-risks-and-gaps.md R1: "skip + report file-level errors; do not
// abort the whole run").
type ProviderError struct {
	Provider string
	Err      error
}

func (e ProviderError) Error() string { return e.Provider + ": " + e.Err.Error() }

// mergeEndpoints implements the group-and-merge step of
// docs/07-discovery.md section 7: group by (method, canonical path), merge
// sources, pick the richest Spec (OpenAPI first), keep the first-seen
// display path form.
func mergeEndpoints(endpoints []domain.Endpoint) []domain.Endpoint {
	type group struct {
		endpoint domain.Endpoint
		sources  map[string]bool
	}
	groups := make(map[domain.EndpointID]*group)
	var order []domain.EndpointID

	for _, ep := range endpoints {
		id := domain.NewEndpointID(ep.Method, ep.Path)
		g, exists := groups[id]
		if !exists {
			g = &group{endpoint: ep, sources: map[string]bool{}}
			g.endpoint.ID = id
			g.endpoint.Path = domain.NormalizePath(ep.Path) // canonical form drives identity; display below preserves richer source's raw form if present
			g.endpoint.Sources = nil
			groups[id] = g
			order = append(order, id)
		}
		for _, s := range ep.Sources {
			g.sources[s] = true
		}
		if len(ep.Sources) == 0 {
			// Providers are expected to set Sources, but guard anyway.
			g.sources[""] = true
		}

		// Richest Spec wins: lower priority number (openapi < express <
		// watch) replaces a less-rich one. Also keep the first-seen
		// display path form from the highest-priority source.
		currentPriority := minPriority(g.endpoint.Sources, g.endpoint.PrimarySource)
		candidatePriority := minPriority(ep.Sources, primarySourceOf(ep))
		if g.endpoint.Spec == nil && ep.Spec != nil {
			g.endpoint.Spec = ep.Spec
		}
		if candidatePriority < currentPriority {
			g.endpoint.Spec = ep.Spec
			g.endpoint.PrimarySource = primarySourceOf(ep)
			// Prefer the richer source's display path casing/params too.
			g.endpoint.Path = ep.Path
		}
		if len(g.endpoint.Tags) == 0 {
			g.endpoint.Tags = ep.Tags
		}
	}

	out := make([]domain.Endpoint, 0, len(order))
	for _, id := range order {
		g := groups[id]
		sources := make([]string, 0, len(g.sources))
		for s := range g.sources {
			if s != "" {
				sources = append(sources, s)
			}
		}
		sort.Slice(sources, func(i, j int) bool { return priorityOf(sources[i]) < priorityOf(sources[j]) })
		g.endpoint.Sources = sources
		if g.endpoint.PrimarySource == "" && len(sources) > 0 {
			g.endpoint.PrimarySource = sources[0]
		}
		out = append(out, g.endpoint)
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func primarySourceOf(ep domain.Endpoint) string {
	if ep.PrimarySource != "" {
		return ep.PrimarySource
	}
	if len(ep.Sources) > 0 {
		return ep.Sources[0]
	}
	return ""
}

func minPriority(sources []string, primary string) int {
	best := priorityOf(primary)
	for _, s := range sources {
		if p := priorityOf(s); p < best {
			best = p
		}
	}
	return best
}
