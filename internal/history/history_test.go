package history

import (
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func ex(url string) domain.Exchange {
	return domain.Exchange{Request: domain.HTTPRequest{Method: "GET", URL: url}}
}

func TestAppend_AssignsIncrementingDisplayIDs(t *testing.T) {
	s := NewMemoryStore(10)
	a := s.Append(ex("/a"))
	b := s.Append(ex("/b"))
	if a.Display != 1 || b.Display != 2 {
		t.Errorf("Display IDs = %d, %d, want 1, 2", a.Display, b.Display)
	}
	if a.ID == "" || b.ID == "" {
		t.Error("expected non-empty ExchangeID (UUID) to be assigned")
	}
	if a.ID == b.ID {
		t.Error("expected distinct UUIDs")
	}
}

func TestAppend_PreservesExistingID(t *testing.T) {
	s := NewMemoryStore(10)
	in := ex("/a")
	in.ID = "custom-id"
	out := s.Append(in)
	if out.ID != "custom-id" {
		t.Errorf("ID = %q, want preserved custom-id", out.ID)
	}
}

func TestGet_FindsByDisplayID(t *testing.T) {
	s := NewMemoryStore(10)
	appended := s.Append(ex("/a"))
	got, ok := s.Get(int(appended.Display))
	if !ok || got.Request.URL != "/a" {
		t.Errorf("Get failed: ok=%v got=%+v", ok, got)
	}
}

func TestGetUUID_FindsByExchangeID(t *testing.T) {
	s := NewMemoryStore(10)
	appended := s.Append(ex("/a"))
	got, ok := s.GetUUID(appended.ID)
	if !ok || got.Request.URL != "/a" {
		t.Errorf("GetUUID failed: ok=%v got=%+v", ok, got)
	}
}

func TestRingBuffer_EvictsOldestWhenFull(t *testing.T) {
	s := NewMemoryStore(3)
	for i := 0; i < 5; i++ {
		s.Append(ex("/x"))
	}
	all := s.List(0)
	if len(all) != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", len(all))
	}
	// The oldest two (display IDs 1, 2) should be gone; 3, 4, 5 remain.
	if all[0].Display != 3 || all[2].Display != 5 {
		t.Errorf("unexpected surviving entries: %+v", all)
	}
	if _, ok := s.Get(1); ok {
		t.Error("expected evicted entry #1 to be gone")
	}
}

func TestList_LimitReturnsMostRecent(t *testing.T) {
	s := NewMemoryStore(10)
	for i := 0; i < 5; i++ {
		s.Append(ex("/x"))
	}
	got := s.List(2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
	if got[0].Display != 4 || got[1].Display != 5 {
		t.Errorf("expected the last 2 entries, got %+v", got)
	}
}

func TestClear_RemovesEverything(t *testing.T) {
	s := NewMemoryStore(10)
	s.Append(ex("/a"))
	s.Clear()
	if len(s.List(0)) != 0 {
		t.Error("expected empty history after Clear")
	}
}

func TestNewMemoryStore_DefaultCapacityWhenZero(t *testing.T) {
	s := NewMemoryStore(0)
	rs := s.(*ringStore)
	if rs.maxEntries != DefaultMaxEntries {
		t.Errorf("maxEntries = %d, want default %d", rs.maxEntries, DefaultMaxEntries)
	}
}
