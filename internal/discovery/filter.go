package discovery

import (
	"path"

	"github.com/sandeepv/apilens/internal/domain"
)

// FilterOptions implements plan.md v8's "Test tags, suites, and ignore
// filters at discover time" — distinct from run-time filtering
// (app.RunFilter's --tag), this tags/excludes *registry entries* the
// moment they're discovered, so `apilens list --tag` and generated tests
// inherit the grouping without every test author repeating it by hand.
type FilterOptions struct {
	// Ignore lists glob patterns (path.Match syntax: "*" and "?" and
	// "[...]", matched against the endpoint's normalized path) — any
	// endpoint whose path matches one of these is dropped from the
	// discover result entirely, as if the provider never found it.
	Ignore []string
	// Tag maps a glob pattern to a tag name; every endpoint whose
	// normalized path matches the pattern gets that tag appended (no
	// duplicates). Applied after Ignore, so a tag pattern can't resurrect
	// an ignored endpoint.
	Tag map[string]string
}

// ApplyFilters implements the ignore-then-tag pass described above. It
// never mutates the input slice's backing array — callers get back a new
// slice with new/updated Endpoint values.
func ApplyFilters(endpoints []domain.Endpoint, opts FilterOptions) []domain.Endpoint {
	if len(opts.Ignore) == 0 && len(opts.Tag) == 0 {
		return endpoints
	}

	out := make([]domain.Endpoint, 0, len(endpoints))
	for _, ep := range endpoints {
		if matchesAny(ep.Path, opts.Ignore) {
			continue
		}
		out = append(out, applyTags(ep, opts.Tag))
	}
	return out
}

func matchesAny(epPath string, patterns []string) bool {
	normalized := domain.NormalizePath(epPath)
	for _, pattern := range patterns {
		if ok, _ := path.Match(pattern, normalized); ok {
			return true
		}
		// Also try the raw (pre-normalization) path — a pattern like
		// "/api/users/:id" written by hand should still match even though
		// the stored Path may carry a different param name.
		if ok, _ := path.Match(pattern, epPath); ok {
			return true
		}
	}
	return false
}

func applyTags(ep domain.Endpoint, tagPatterns map[string]string) domain.Endpoint {
	if len(tagPatterns) == 0 {
		return ep
	}
	normalized := domain.NormalizePath(ep.Path)
	existing := map[string]bool{}
	for _, t := range ep.Tags {
		existing[t] = true
	}
	tags := append([]string(nil), ep.Tags...)
	for pattern, tag := range tagPatterns {
		matched, _ := path.Match(pattern, normalized)
		if !matched {
			matched, _ = path.Match(pattern, ep.Path)
		}
		if matched && !existing[tag] {
			tags = append(tags, tag)
			existing[tag] = true
		}
	}
	ep.Tags = tags
	return ep
}
