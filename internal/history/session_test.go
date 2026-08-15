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
		Timing:   domain.Timing{Duration: 42 * time.Millisecond},
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
}
