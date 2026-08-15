package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

// newChainingTestApp builds an App over a fresh project dir with a "local"
// environment pointing at ts, and copies the given fixture files into
// .apilens/tests (a real testdef.Loader.LoadAll walk, not an in-memory
// shortcut, so this exercises the full compile+run path).
func newChainingTestApp(t *testing.T, ts *httptest.Server, fixtureFiles ...string) *App {
	t.Helper()
	dir := t.TempDir()

	envDir := filepath.Join(dir, ".apilens", "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir envDir: %v", err)
	}
	envYAML := "base_url: " + ts.URL + "\n"
	if err := os.WriteFile(filepath.Join(envDir, "local.yaml"), []byte(envYAML), 0o644); err != nil {
		t.Fatalf("writing local.yaml: %v", err)
	}

	testsDir := filepath.Join(dir, ".apilens", "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatalf("mkdir testsDir: %v", err)
	}
	for _, f := range fixtureFiles {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading fixture %s: %v", f, err)
		}
		if err := os.WriteFile(filepath.Join(testsDir, filepath.Base(f)), data, 0o644); err != nil {
			t.Fatalf("writing fixture %s: %v", f, err)
		}
	}

	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Env.Use("local"); err != nil {
		t.Fatalf("Env.Use(local): %v", err)
	}
	return a
}

// chainingFixtureServer mimics the fixture-server's /api/users POST/GET
// shape closely enough for the chaining fixtures (which read
// {{responses.login.body.data.id}}) to exercise a real created-then-fetched
// flow.
func chainingFixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	type user struct {
		ID    int    `json:"id"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	var created user
	mux := http.NewServeMux()
	mux.HandleFunc("/api/users", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Name, Email string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		created = user{ID: 7, Name: body.Name, Email: body.Email}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
	})
	mux.HandleFunc("/api/users/7", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": created})
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func TestRunSuite_ChainedSuitePassesLoginThenFetchesCreatedUser(t *testing.T) {
	ts := chainingFixtureServer(t)
	defer ts.Close()

	a := newChainingTestApp(t, ts,
		"../../testdata/tests/chaining/valid/01-login.yaml",
		"../../testdata/tests/chaining/valid/02-fetch-created.yaml",
	)

	report, err := a.RunSuite(context.Background(), RunFilter{}, nil)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if report.Counts.Failed != 0 || report.Counts.Errored != 0 {
		t.Fatalf("expected a clean pass, got %+v results=%+v", report.Counts, report.Results)
	}
	if report.Counts.Passed != 2 {
		t.Fatalf("expected 2 passed, got %+v", report.Counts)
	}
}

func TestRunSuite_ChainedSuiteForcesSequentialEvenIfParallelRequested(t *testing.T) {
	ts := chainingFixtureServer(t)
	defer ts.Close()

	a := newChainingTestApp(t, ts,
		"../../testdata/tests/chaining/valid/01-login.yaml",
		"../../testdata/tests/chaining/valid/02-fetch-created.yaml",
	)

	// If chaining did NOT force sequential, this would race and could
	// fail intermittently (fetch might run before login populates the
	// response store). Running it against --parallel proves the
	// override — a flaky failure here would indicate the override isn't
	// working, not just bad luck, since login always executes first in
	// deterministic (file-path-sorted) sequential order.
	report, err := a.RunSuite(context.Background(), RunFilter{Parallel: true}, nil)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if report.Counts.Failed != 0 || report.Counts.Errored != 0 {
		t.Fatalf("expected a clean pass even with --parallel requested, got %+v results=%+v", report.Counts, report.Results)
	}
}

func TestRunSuite_DuplicateTestIDIsConfigError(t *testing.T) {
	ts := chainingFixtureServer(t)
	defer ts.Close()

	a := newChainingTestApp(t, ts,
		"../../testdata/tests/chaining/invalid-duplicate-id/a.yaml",
		"../../testdata/tests/chaining/invalid-duplicate-id/b.yaml",
	)

	_, err := a.RunSuite(context.Background(), RunFilter{}, nil)
	if err == nil {
		t.Fatal("expected a config error for duplicate test ids within one suite")
	}
}

func TestRunSuite_UnresolvedChainReferenceErrorsThatTestOnly(t *testing.T) {
	ts := chainingFixtureServer(t)
	defer ts.Close()

	a := newChainingTestApp(t, ts,
		"../../testdata/tests/chaining/unresolved-reference/references-unrun-test.yaml",
	)

	report, err := a.RunSuite(context.Background(), RunFilter{}, nil)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if report.Counts.Errored != 1 {
		t.Fatalf("expected the single test to error (unresolved response reference), got %+v", report.Counts)
	}
	if report.Results[0].Status != domain.StatusErrored {
		t.Errorf("Status = %s", report.Results[0].Status)
	}
}
