// Package discovery implements the provider port and orchestrator from
// docs/07-discovery.md and docs/04-interfaces.md section 2. Providers only
// answer "does this look like mine?" and "what endpoints exist?" — the
// orchestrator owns merging, deduping, and registry writes
// (docs/03-plugins.md section 4: "Providers return domain Endpoint values.
// They do not write the registry").
package discovery

import (
	"context"
	"io/fs"

	"github.com/sandeepv/apilens/internal/domain"
)

// Options configures a Discover call (docs/04-interfaces.md section 2).
type Options struct {
	// Enabled lists provider names to run; empty means "all enabled in
	// config". --source on the CLI maps here.
	Enabled []string
	// Paths are explicit spec/source paths that bypass Detect and force
	// a provider to read them directly (--path on the CLI).
	Paths []string
}

// Provider is the discovery plugin contract (docs/03-plugins.md section 4,
// docs/04-interfaces.md section 2).
type Provider interface {
	Name() string
	Detect(ctx context.Context, root fs.FS) (bool, error)
	Discover(ctx context.Context, root fs.FS, opts Options) ([]domain.Endpoint, error)
}

// enabled reports whether name should run given Options.Enabled (empty
// means everything not explicitly disabled by config is eligible; the
// orchestrator is also given a disabledByConfig set separately).
func enabled(name string, list []string) bool {
	if len(list) == 0 {
		return true
	}
	for _, n := range list {
		if n == name {
			return true
		}
	}
	return false
}
