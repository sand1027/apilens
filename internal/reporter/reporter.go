// Package reporter formats a domain.Report. Reporters never decide
// pass/fail — that is computed by internal/testrunner (ADR-022).
package reporter

import "github.com/sandeepv/apilens/internal/domain"

// Reporter implements docs/04-interfaces.md section 9 / docs/03-plugins.md
// section 5.
type Reporter interface {
	Name() string
	Formats() []string
	Start(meta domain.SuiteMeta)
	TestFinished(result domain.TestResult)
	SuiteFinished(report domain.Report) error
}

// Registry looks up a Reporter by the `--format` name.
type Registry struct {
	reporters map[string]Reporter
}

// NewRegistry builds an empty registry.
func NewRegistry() *Registry {
	return &Registry{reporters: make(map[string]Reporter)}
}

// Register adds a reporter under each of its Formats().
func (r *Registry) Register(rep Reporter) {
	for _, f := range rep.Formats() {
		r.reporters[f] = rep
	}
}

// Get looks up a reporter by format name ("terminal", "json").
func (r *Registry) Get(format string) (Reporter, bool) {
	rep, ok := r.reporters[format]
	return rep, ok
}
