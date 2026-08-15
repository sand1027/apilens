package graph

import (
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func exAt(method, urlPath string, start time.Time) domain.Exchange {
	return domain.Exchange{
		Request: domain.HTTPRequest{Method: domain.Method(method), URL: urlPath},
		Timing:  domain.Timing{Start: start},
	}
}

func TestBuild_ChainsWithinWindowIntoEdges(t *testing.T) {
	base := time.Now()
	exchanges := []domain.Exchange{
		exAt("POST", "http://x/login", base),
		exAt("GET", "http://x/api/users", base.Add(100*time.Millisecond)),
	}
	g := Build(exchanges, 2*time.Second)
	if len(g.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d: %+v", len(g.Nodes), g.Nodes)
	}
	if len(g.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %+v", len(g.Edges), g.Edges)
	}
	edge := g.Edges[0]
	if edge.From.Path != "/login" || edge.To.Path != "/api/users" {
		t.Errorf("unexpected edge: %+v", edge)
	}
	if edge.Weight != 1 {
		t.Errorf("expected weight 1, got %d", edge.Weight)
	}
}

func TestBuild_GapLargerThanWindowStartsNewBurst(t *testing.T) {
	base := time.Now()
	exchanges := []domain.Exchange{
		exAt("POST", "http://x/login", base),
		exAt("GET", "http://x/api/users", base.Add(10*time.Second)), // far beyond window
	}
	g := Build(exchanges, 2*time.Second)
	if len(g.Edges) != 0 {
		t.Errorf("expected no edge across a large time gap, got %+v", g.Edges)
	}
}

func TestBuild_RepeatedSequenceIncreasesWeight(t *testing.T) {
	base := time.Now()
	exchanges := []domain.Exchange{
		exAt("POST", "http://x/login", base),
		exAt("GET", "http://x/api/users", base.Add(100*time.Millisecond)),
		exAt("POST", "http://x/login", base.Add(5*time.Second)),
		exAt("GET", "http://x/api/users", base.Add(5100*time.Millisecond)),
	}
	g := Build(exchanges, 2*time.Second)
	if len(g.Edges) != 1 {
		t.Fatalf("expected the two bursts to merge into 1 edge, got %d: %+v", len(g.Edges), g.Edges)
	}
	if g.Edges[0].Weight != 2 {
		t.Errorf("expected weight 2 for a sequence observed twice, got %d", g.Edges[0].Weight)
	}
}

func TestBuild_NoSelfLoopForRepeatedCallInSameBurst(t *testing.T) {
	base := time.Now()
	exchanges := []domain.Exchange{
		exAt("GET", "http://x/api/users", base),
		exAt("GET", "http://x/api/users", base.Add(10*time.Millisecond)),
	}
	g := Build(exchanges, 2*time.Second)
	if len(g.Edges) != 0 {
		t.Errorf("expected no self-loop edge for identical repeated calls, got %+v", g.Edges)
	}
	if len(g.Nodes) != 1 {
		t.Errorf("expected 1 distinct node, got %+v", g.Nodes)
	}
}

func TestBuild_EmptyInputProducesEmptyGraph(t *testing.T) {
	g := Build(nil, 0)
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("expected empty graph, got nodes=%+v edges=%+v", g.Nodes, g.Edges)
	}
}

func TestBuild_ZeroWindowFallsBackToDefault(t *testing.T) {
	base := time.Now()
	exchanges := []domain.Exchange{
		exAt("POST", "http://x/login", base),
		exAt("GET", "http://x/api/users", base.Add(500*time.Millisecond)),
	}
	g := Build(exchanges, 0)
	if len(g.Edges) != 1 {
		t.Errorf("expected default window (2s) to still connect a 500ms gap, got %+v", g.Edges)
	}
}
