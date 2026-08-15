// Package registry implements the endpoint store from
// docs/04-interfaces.md section 3. It is a replaceable cache, not a
// hand-edited source of truth (ADR-017) — discover overwrites it, list and
// inspect read it.
package registry

import (
	"sort"
	"strings"
	"sync"

	"github.com/sandeepv/apilens/internal/domain"
)

// Filter narrows List results (docs/04-interfaces.md section 3).
type Filter struct {
	Method string
	Path   string // glob or prefix; here implemented as substring match
	Tag    string
	Source string
}

// Store implements docs/04-interfaces.md section 3.
type Store interface {
	Replace(endpoints []domain.Endpoint) error
	Upsert(endpoint domain.Endpoint) error
	List(filter Filter) []domain.Endpoint
	Get(method domain.Method, path string) (domain.Endpoint, bool)
	GetByID(id domain.EndpointID) (domain.Endpoint, bool)
}

// memoryStore is the MVP in-memory implementation
// (docs/04-interfaces.md section 3: "MVP store is in-memory, optionally
// hydrated from the last discover write").
type memoryStore struct {
	mu        sync.RWMutex
	endpoints map[domain.EndpointID]domain.Endpoint
}

// NewMemoryStore builds an empty in-memory Store.
func NewMemoryStore() Store {
	return &memoryStore{endpoints: make(map[domain.EndpointID]domain.Endpoint)}
}

func (s *memoryStore) Replace(endpoints []domain.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpoints = make(map[domain.EndpointID]domain.Endpoint, len(endpoints))
	for _, ep := range endpoints {
		s.endpoints[idOf(ep)] = ep
	}
	return nil
}

func (s *memoryStore) Upsert(endpoint domain.Endpoint) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := idOf(endpoint)
	if existing, ok := s.endpoints[id]; ok {
		endpoint = mergeUpsert(existing, endpoint)
	}
	s.endpoints[id] = endpoint
	return nil
}

// mergeUpsert combines an existing endpoint with a newly observed one
// (used by watch in v3 to enrich without clobbering a richer OpenAPI spec —
// docs/07-discovery.md section 2: "Watch-discovered routes ... enrich the
// registry but do not overwrite OpenAPI schemas").
func mergeUpsert(existing, incoming domain.Endpoint) domain.Endpoint {
	merged := existing
	sources := map[string]bool{}
	for _, s := range existing.Sources {
		sources[s] = true
	}
	for _, s := range incoming.Sources {
		sources[s] = true
	}
	merged.Sources = sortedKeys(sources)
	if existing.Spec == nil && incoming.Spec != nil {
		merged.Spec = incoming.Spec
		merged.PrimarySource = incoming.PrimarySource
	}
	if len(existing.Tags) == 0 {
		merged.Tags = incoming.Tags
	}
	return merged
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *memoryStore) List(filter Filter) []domain.Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]domain.Endpoint, 0, len(s.endpoints))
	for _, ep := range s.endpoints {
		if !matches(ep, filter) {
			continue
		}
		out = append(out, ep)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Method < out[j].Method
	})
	return out
}

func matches(ep domain.Endpoint, f Filter) bool {
	if f.Method != "" && string(ep.Method) != string(domain.NormalizeMethod(f.Method)) {
		return false
	}
	if f.Path != "" && !strings.Contains(ep.Path, f.Path) {
		return false
	}
	if f.Tag != "" && !hasTag(ep.Tags, f.Tag) {
		return false
	}
	if f.Source != "" && !hasSource(ep.Sources, f.Source) {
		return false
	}
	return true
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

func hasSource(sources []string, source string) bool {
	for _, s := range sources {
		if s == source {
			return true
		}
	}
	return false
}

func (s *memoryStore) Get(method domain.Method, path string) (domain.Endpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	id := domain.NewEndpointID(method, path)
	ep, ok := s.endpoints[id]
	return ep, ok
}

func (s *memoryStore) GetByID(id domain.EndpointID) (domain.Endpoint, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ep, ok := s.endpoints[id]
	return ep, ok
}

func idOf(ep domain.Endpoint) domain.EndpointID {
	if ep.ID != "" {
		return ep.ID
	}
	return domain.NewEndpointID(ep.Method, ep.Path)
}
