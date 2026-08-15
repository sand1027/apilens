package app

import (
	"time"

	"github.com/sandeepv/apilens/internal/graph"
)

// GraphOptions configures Graph.
type GraphOptions struct {
	// WindowMS overrides graph.DefaultWindow; 0 uses the default.
	WindowMS int
}

// Graph builds an inferred API dependency graph from history (in-memory
// if `watch` is running in this process, otherwise the session JSONL
// file — same source HistoryList already uses). Implements plan.md v8's
// "API dependency graphs": this is captured-traffic-derived, not a static
// analysis of application source, so it can only ever be a hint (see
// internal/graph's package doc for the honesty note this mirrors
// docs/11-risks-and-gaps.md G19 for page-to-API mapping).
func (a *App) Graph(opts GraphOptions) graph.Graph {
	exchanges := a.historyOrEmpty()
	window := graph.DefaultWindow
	if opts.WindowMS > 0 {
		window = time.Duration(opts.WindowMS) * time.Millisecond
	}
	return graph.Build(exchanges, window)
}
