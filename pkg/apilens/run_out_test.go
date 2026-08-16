package apilens

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newOutTestProject builds a real .apilens/ project on disk (env pointed
// at ts, one passing GET /health test) so Engine.Run's --out path is
// exercised through the same New()/Run() surface the CLI actually calls,
// not an internal shortcut.
func newOutTestProject(t *testing.T, ts *httptest.Server) (Engine, string) {
	t.Helper()
	dir := t.TempDir()

	envDir := filepath.Join(dir, ".apilens", "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatalf("mkdir envDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(envDir, "local.yaml"), []byte("base_url: "+ts.URL+"\n"), 0o644); err != nil {
		t.Fatalf("writing local.yaml: %v", err)
	}

	testsDir := filepath.Join(dir, ".apilens", "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatalf("mkdir testsDir: %v", err)
	}
	testYAML := `version: 1
name: Health
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  status:
    equals: 200
`
	if err := os.WriteFile(filepath.Join(testsDir, "health.yaml"), []byte(testYAML), 0o644); err != nil {
		t.Fatalf("writing health.yaml: %v", err)
	}

	eng, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := eng.UseEnv("local"); err != nil {
		t.Fatalf("UseEnv: %v", err)
	}
	return eng, dir
}

func healthTestServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
}

func TestRun_OutWritesJSONFileByExtension(t *testing.T) {
	ts := healthTestServer()
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	outPath := filepath.Join(dir, ".apilens", "reports", "latest.json")
	report, err := eng.Run(context.Background(), RunFilter{Out: outPath})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Passed != 1 {
		t.Fatalf("expected 1 passed test, got %+v", report.Counts)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected --out file to exist: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty report file")
	}
	if !contains(string(data), `"status": "passed"`) {
		t.Errorf("expected the JSON report to show a passed test, got:\n%s", data)
	}
}

func TestRun_OutWritesJUnitByXMLExtension(t *testing.T) {
	ts := healthTestServer()
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	outPath := filepath.Join(dir, ".apilens", "reports", "latest.xml")
	if _, err := eng.Run(context.Background(), RunFilter{Out: outPath}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected --out file to exist: %v", err)
	}
	if !contains(string(data), "<testsuites>") {
		t.Errorf("expected JUnit XML content, got:\n%s", data)
	}
}

func TestRun_OutFormatOverridesExtensionInference(t *testing.T) {
	ts := healthTestServer()
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	// A ".txt" extension would otherwise infer json; force junit.
	outPath := filepath.Join(dir, ".apilens", "reports", "latest.txt")
	if _, err := eng.Run(context.Background(), RunFilter{Out: outPath, OutFormat: "junit"}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("expected --out file to exist: %v", err)
	}
	if !contains(string(data), "<testsuites>") {
		t.Errorf("expected --out-format=junit to win over extension inference, got:\n%s", data)
	}
}

func TestRun_OutStillPrintsToStdoutReporterIndependently(t *testing.T) {
	// The point of --out is "in addition to", not "instead of" --
	// ReporterFormat (stdout) and Out (file) are independent knobs.
	ts := healthTestServer()
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	outPath := filepath.Join(dir, ".apilens", "reports", "latest.json")
	report, err := eng.Run(context.Background(), RunFilter{Out: outPath, ReporterFormat: "terminal"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Tests != 1 {
		t.Errorf("expected the report to still be computed normally, got %+v", report.Counts)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected the file to be written even when ReporterFormat is terminal: %v", err)
	}
}

func TestRun_OutCreatesParentDirectoryIfMissing(t *testing.T) {
	ts := healthTestServer()
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	// "reports" does not exist yet under this fresh project dir.
	outPath := filepath.Join(dir, ".apilens", "reports", "nested", "latest.json")
	if _, err := eng.Run(context.Background(), RunFilter{Out: outPath}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Errorf("expected --out to create missing parent directories: %v", err)
	}
}

func TestRun_OutCapturesFailedTestsToo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	eng, dir := newOutTestProject(t, ts)

	outPath := filepath.Join(dir, ".apilens", "reports", "latest.json")
	report, err := eng.Run(context.Background(), RunFilter{Out: outPath})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.Counts.Failed != 1 {
		t.Fatalf("expected the test to fail (server returns 500), got %+v", report.Counts)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("reading --out file: %v", err)
	}
	if !contains(string(data), `"status": "failed"`) {
		t.Errorf("expected the file report to show the failure, got:\n%s", data)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
