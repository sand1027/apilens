// Package graph implements plan.md v8's "API dependency graphs". ApiLens
// has no access to the application's source-level call graph — the only
// evidence it can observe is captured HTTP traffic (internal/history), so
// this package builds an INFERRED graph: an edge A -> B means "B was
// observed shortly after A within the same watch session", which is a
// proxy for "B may depend on A" (e.g. a login call followed by an
// authenticated call, or a create followed by a lookup of the created
// resource). This is explicitly a heuristic, not ground truth — the same
// honesty standard docs/11-risks-and-gaps.md G19 holds page-to-API
// mapping to.
package graph

import (
	"sort"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

// DefaultWindow is how close in time two exchanges must be to be
// considered a candidate dependency edge (docs/08-proxy.md's traffic is
// typically bursty within a single user action — a few hundred ms to a
// couple seconds covers most XHR fan-out patterns without linking
// unrelated later requests).
const DefaultWindow = 2 * time.Second

// Node identifies one endpoint in the graph (its canonical identity —
// domain.NewEndpointID's inputs — not the full domain.Endpoint, so the
// graph stays independent of registry enrichment).
type Node struct {
	Method domain.Method
	Path   string // canonical (domain.NormalizePath) form
}

// Edge is a directed, weighted "B observed shortly after A" relationship.
type Edge struct {
	From   Node
	To     Node
	Weight int // number of times this A->B sequence was observed
}

// Graph is the full inferred dependency graph for a set of captured
// exchanges.
type Graph struct {
	Nodes []Node
	Edges []Edge
}

// Build groups exchanges by time proximity (within window of each other,
// chained — not just pairwise from the first) and adds a directed edge
// from every earlier exchange in a burst to every later one, weighted by
// how many times that exact (from, to) pair recurs across all observed
// bursts. Exchanges are assumed sorted oldest-first, matching
// history.Store.List's contract.
func Build(exchanges []domain.Exchange, window time.Duration) Graph {
	if window <= 0 {
		window = DefaultWindow
	}

	nodeSet := map[Node]bool{}
	edgeWeights := map[[2]Node]int{}

	bursts := groupIntoBursts(exchanges, window)
	for _, burst := range bursts {
		nodes := make([]Node, 0, len(burst))
		for _, ex := range burst {
			n := nodeFor(ex)
			nodes = append(nodes, n)
			nodeSet[n] = true
		}
		for i := 0; i < len(nodes); i++ {
			for j := i + 1; j < len(nodes); j++ {
				if nodes[i] == nodes[j] {
					continue // no self-loops from repeated identical calls in one burst
				}
				key := [2]Node{nodes[i], nodes[j]}
				edgeWeights[key]++
			}
		}
	}

	g := Graph{}
	for n := range nodeSet {
		g.Nodes = append(g.Nodes, n)
	}
	sort.Slice(g.Nodes, func(i, j int) bool {
		if g.Nodes[i].Path != g.Nodes[j].Path {
			return g.Nodes[i].Path < g.Nodes[j].Path
		}
		return g.Nodes[i].Method < g.Nodes[j].Method
	})

	for pair, weight := range edgeWeights {
		g.Edges = append(g.Edges, Edge{From: pair[0], To: pair[1], Weight: weight})
	}
	sort.Slice(g.Edges, func(i, j int) bool {
		if g.Edges[i].From != g.Edges[j].From {
			return nodeLess(g.Edges[i].From, g.Edges[j].From)
		}
		return nodeLess(g.Edges[i].To, g.Edges[j].To)
	})

	return g
}

func nodeLess(a, b Node) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Method < b.Method
}

func nodeFor(ex domain.Exchange) Node {
	return Node{Method: ex.Request.Method, Path: domain.NormalizePath(requestPath(ex.Request.URL))}
}

// groupIntoBursts splits exchanges (assumed oldest-first) into
// consecutive runs where each exchange starts within window of the
// previous one's start — a simple "gap" clustering, not a fixed-size
// sliding window, so a long quiet gap always starts a new burst.
func groupIntoBursts(exchanges []domain.Exchange, window time.Duration) [][]domain.Exchange {
	var bursts [][]domain.Exchange
	var current []domain.Exchange
	var last time.Time

	for _, ex := range exchanges {
		ts := ex.Timing.Start
		if ts.IsZero() {
			ts = ex.Timestamp
		}
		if len(current) > 0 && ts.Sub(last) > window {
			bursts = append(bursts, current)
			current = nil
		}
		current = append(current, ex)
		last = ts
	}
	if len(current) > 0 {
		bursts = append(bursts, current)
	}
	return bursts
}
