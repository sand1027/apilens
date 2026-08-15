// Package history implements the in-memory exchange log from
// docs/04-interfaces.md section 10 and the session JSONL file from
// docs/08-proxy.md section 3 ("Cross-terminal replay"). History is
// session-scoped: display IDs are not durable across restarts
// (ADR-004, ADR-013, docs/11-risks-and-gaps.md G7).
package history

import (
	"sync"

	"github.com/google/uuid"
	"github.com/sandeepv/apilens/internal/domain"
)

// DefaultMaxEntries mirrors docs/04-interfaces.md section 10:
// "Default capacity is configurable (history.max_entries, default 1000).
// Oldest dropped first."
const DefaultMaxEntries = 1000

// Store implements docs/04-interfaces.md section 10.
type Store interface {
	// Append assigns a DisplayID and ExchangeID (if not already set) and
	// stores the exchange, evicting the oldest entry if at capacity.
	Append(ex domain.Exchange) domain.Exchange
	Get(displayID int) (domain.Exchange, bool)
	GetUUID(id domain.ExchangeID) (domain.Exchange, bool)
	List(limit int) []domain.Exchange
	Clear()
}

// ringStore is the MVP in-memory ring buffer implementation.
type ringStore struct {
	mu         sync.Mutex
	maxEntries int
	entries    []domain.Exchange        // oldest first
	byDisplay  map[domain.DisplayID]int // index into entries, kept in sync on evict
	byUUID     map[domain.ExchangeID]int
	nextID     domain.DisplayID
}

// NewMemoryStore builds a ring buffer capped at maxEntries (<=0 uses
// DefaultMaxEntries).
func NewMemoryStore(maxEntries int) Store {
	if maxEntries <= 0 {
		maxEntries = DefaultMaxEntries
	}
	return &ringStore{
		maxEntries: maxEntries,
		byDisplay:  make(map[domain.DisplayID]int),
		byUUID:     make(map[domain.ExchangeID]int),
		nextID:     1,
	}
}

func (s *ringStore) Append(ex domain.Exchange) domain.Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()

	if ex.ID == "" {
		ex.ID = domain.ExchangeID(uuid.NewString())
	}
	ex.Display = s.nextID
	s.nextID++

	s.entries = append(s.entries, ex)
	if len(s.entries) > s.maxEntries {
		s.entries = s.entries[1:] // drop oldest
		s.reindex()
	} else {
		idx := len(s.entries) - 1
		s.byDisplay[ex.Display] = idx
		s.byUUID[ex.ID] = idx
	}
	return ex
}

// reindex rebuilds the lookup maps after an eviction shifts every index.
func (s *ringStore) reindex() {
	s.byDisplay = make(map[domain.DisplayID]int, len(s.entries))
	s.byUUID = make(map[domain.ExchangeID]int, len(s.entries))
	for i, e := range s.entries {
		s.byDisplay[e.Display] = i
		s.byUUID[e.ID] = i
	}
}

func (s *ringStore) Get(displayID int) (domain.Exchange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.byDisplay[domain.DisplayID(displayID)]
	if !ok {
		return domain.Exchange{}, false
	}
	return s.entries[idx], true
}

func (s *ringStore) GetUUID(id domain.ExchangeID) (domain.Exchange, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, ok := s.byUUID[id]
	if !ok {
		return domain.Exchange{}, false
	}
	return s.entries[idx], true
}

// List returns the most recent `limit` exchanges, newest last (matching
// the watch terminal UI's append-only feed). limit<=0 returns everything.
func (s *ringStore) List(limit int) []domain.Exchange {
	s.mu.Lock()
	defer s.mu.Unlock()
	if limit <= 0 || limit >= len(s.entries) {
		out := make([]domain.Exchange, len(s.entries))
		copy(out, s.entries)
		return out
	}
	start := len(s.entries) - limit
	out := make([]domain.Exchange, limit)
	copy(out, s.entries[start:])
	return out
}

func (s *ringStore) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = nil
	s.byDisplay = make(map[domain.DisplayID]int)
	s.byUUID = make(map[domain.ExchangeID]int)
}
