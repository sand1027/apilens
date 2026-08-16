package history

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestNewSessionFile_CreatesFileWithMode0600(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	sf, err := NewSessionFile(path)
	if err != nil {
		t.Fatalf("NewSessionFile: %v", err)
	}
	if sf.Path() != path {
		t.Errorf("Path() = %q, want %q", sf.Path(), path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
}

func TestAppend_ThenReadAll_RoundTrips(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	sf, err := NewSessionFile(path)
	if err != nil {
		t.Fatalf("NewSessionFile: %v", err)
	}

	h := http.Header{}
	h.Set("Authorization", "Bearer ********")
	ex1 := domain.Exchange{
		Display:  1,
		ID:       "id-1",
		Request:  domain.HTTPRequest{Method: "GET", URL: "/api/users", Headers: h},
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"ok":true}`)},
		Timing:   domain.Timing{Duration: 42 * time.Millisecond, Wait: 30 * time.Millisecond, Transfer: 5 * time.Millisecond},
		Redacted: true,
	}
	ex2 := domain.Exchange{Display: 2, ID: "id-2", Request: domain.HTTPRequest{Method: "POST", URL: "/api/orders"}}

	if err := sf.Append(ex1); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := sf.Append(ex2); err != nil {
		t.Fatalf("Append: %v", err)
	}

	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records, got %d", len(got))
	}
	if got[0].Request.URL != "/api/users" || got[0].Display != 1 {
		t.Errorf("record 0 = %+v", got[0])
	}
	if got[0].Request.Headers.Get("Authorization") != "Bearer ********" {
		t.Errorf("expected redacted header to survive round-trip, got %q", got[0].Request.Headers.Get("Authorization"))
	}
	if got[0].Timing.Wait != 30*time.Millisecond || got[0].Timing.Transfer != 5*time.Millisecond {
		t.Errorf("phase timings = %+v", got[0].Timing)
	}
	if got[1].Request.Method != "POST" {
		t.Errorf("record 1 method = %q", got[1].Request.Method)
	}
}

func TestReadAll_MissingFileReturnsNilNoError(t *testing.T) {
	got, err := ReadAll("/does/not/exist/session.jsonl")
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for missing file, got %v", got)
	}
}

func TestReadAll_SkipsCorruptLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	content := "{\"display_id\":1,\"method\":\"GET\",\"url\":\"/ok\"}\nnot json at all\n{\"display_id\":2,\"method\":\"GET\",\"url\":\"/ok2\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 valid records (corrupt line skipped), got %d: %+v", len(got), got)
	}
}

func TestSearchPaths_IncludesActiveWatchSession(t *testing.T) {
	t.Setenv("APILENS_HISTORY_FILE", "")
	ptr := filepath.Join(t.TempDir(), "ptr")
	t.Setenv("APILENS_HISTORY_POINTER", ptr)
	project := t.TempDir()
	active := filepath.Join(t.TempDir(), "watch-session.jsonl")
	if err := SetActivePath(active); err != nil {
		t.Fatal(err)
	}
	paths := SearchPaths(project)
	if len(paths) != 2 {
		t.Fatalf("SearchPaths = %v", paths)
	}
	if paths[0] != DefaultPath(project) {
		t.Errorf("first path = %q", paths[0])
	}
	if got := ActivePath(); got != paths[1] {
		t.Errorf("active = %q path[1] = %q", got, paths[1])
	}
}

func TestDefaultPath_UsesEnvVarWhenSet(t *testing.T) {
	t.Setenv("APILENS_HISTORY_FILE", "/custom/path.jsonl")
	got := DefaultPath("/some/project")
	if got != "/custom/path.jsonl" {
		t.Errorf("DefaultPath = %q, want the env override", got)
	}
}

func TestDefaultPath_DeterministicPerProjectDir(t *testing.T) {
	t.Setenv("APILENS_HISTORY_FILE", "")
	a := DefaultPath("/some/project")
	b := DefaultPath("/some/project")
	c := DefaultPath("/other/project")
	if a != b {
		t.Errorf("expected same path for same project dir: %q vs %q", a, b)
	}
	if a == c {
		t.Error("expected different paths for different project dirs")
	}
	want := filepath.Join("/some/project", ".apilens", "history", "session.jsonl")
	if a != want {
		t.Errorf("DefaultPath = %q, want %q", a, want)
	}
}

func TestNewSessionFile_CreatesParentDirAndTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".apilens", "history", "session.jsonl")
	if _, err := NewSessionFile(path); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected nested session file: %v", err)
	}
	if err := os.WriteFile(path, []byte("{\"display_id\":1,\"method\":\"GET\",\"url\":\"/stale\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewSessionFile(path); err != nil {
		t.Fatalf("recreate: %v", err)
	}
	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected truncated session, got %d records", len(got))
	}
}

func TestReadAll_SkipsOversizedLineAndKeepsNeighbors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "session.jsonl")
	huge := make([]byte, maxHistoryLine+8)
	for i := range huge {
		huge[i] = 'x'
	}
	content := "{\"display_id\":1,\"method\":\"GET\",\"url\":\"/ok\"}\n" +
		string(huge) + "\n" +
		"{\"display_id\":2,\"method\":\"POST\",\"url\":\"/graphql\"}\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadAll(path)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 records after skipping oversized line, got %d", len(got))
	}
	if got[1].Request.URL != "/graphql" {
		t.Errorf("record 1 url = %q", got[1].Request.URL)
	}
}
