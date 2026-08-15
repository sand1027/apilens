// Package plugins wires built-in implementations into the ports defined by
// docs/03-plugins.md. Registration is explicit — no init() magic (ADR-014).
// v1 only wires reporters and auth schemes; discovery/assertion plugin
// kinds beyond the built-in core assertion engine land in later versions.
package plugins

import (
	"github.com/sandeepv/apilens/internal/auth"
	"github.com/sandeepv/apilens/internal/reporter"
)

// Host is the plugin host described in docs/03-plugins.md section 3. It is
// intentionally narrow in v1: only the ports that v1 features use
// (reporter, auth) are wired. Discovery/assertion-evaluator registration
// arrives with v2 without changing this shape.
type Host struct {
	Reporters *reporter.Registry
	Auth      *auth.Registry
}

// NewHost builds a Host with no plugins registered. Use RegisterBuiltins to
// populate it, mirroring cmd/apilens/main.go's explicit wiring
// (docs/03-plugins.md section 3 sequence diagram).
func NewHost() *Host {
	return &Host{
		Reporters: reporter.NewRegistry(),
		Auth:      auth.NewRegistry(),
	}
}

// RegisterBuiltins registers every built-in plugin: terminal, json, and
// junit reporters (docs/03-plugins.md section 8, plan.md v5 adds junit to
// the Phase 1 terminal/json pair), and bearer/basic/apikey/cookie auth
// schemes.
func RegisterBuiltins(h *Host, terminalRep, jsonRep, junitRep reporter.Reporter) {
	h.Reporters.Register(terminalRep)
	h.Reporters.Register(jsonRep)
	h.Reporters.Register(junitRep)
	auth.RegisterBuiltins(h.Auth)
}
