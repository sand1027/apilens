package recording

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/testdef"
)

func TestWriteSuite_WritesNumberedFilesInOrder(t *testing.T) {
	steps := []Step{
		{ID: "login", Test: domain.TestCase{Version: 2, ID: "login", Name: "Login",
			Request: domain.RequestTemplate{Method: "POST", URL: "{{base_url}}/api/users"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: intPtr(201)}}}},
		{ID: "fetch", Test: domain.TestCase{Version: 2, ID: "fetch", Name: "Fetch",
			Request: domain.RequestTemplate{Method: "GET", URL: "{{base_url}}/api/users/{{responses.login.body.data.id}}"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: intPtr(200)}}}},
	}

	dir := t.TempDir()
	written, err := WriteSuite(steps, WriteOptions{Dir: dir})
	if err != nil {
		t.Fatalf("WriteSuite: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 files written, got %d: %v", len(written), written)
	}
	if filepath.Base(written[0]) != "01-login.yaml" {
		t.Errorf("first file = %s, want 01-login.yaml", filepath.Base(written[0]))
	}
	if filepath.Base(written[1]) != "02-fetch.yaml" {
		t.Errorf("second file = %s, want 02-fetch.yaml", filepath.Base(written[1]))
	}
}

func TestWriteSuite_RefusesOverwriteWithoutForce(t *testing.T) {
	steps := []Step{
		{ID: "a", Test: domain.TestCase{Version: 2, ID: "a", Name: "A",
			Request: domain.RequestTemplate{Method: "GET", URL: "{{base_url}}/health"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: intPtr(200)}}}},
	}
	dir := t.TempDir()
	if _, err := WriteSuite(steps, WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("first WriteSuite: %v", err)
	}
	if _, err := WriteSuite(steps, WriteOptions{Dir: dir}); err == nil {
		t.Fatal("expected an error overwriting existing files without --force")
	}
}

func TestWriteSuite_ForceOverwrites(t *testing.T) {
	steps := []Step{
		{ID: "a", Test: domain.TestCase{Version: 2, ID: "a", Name: "A",
			Request: domain.RequestTemplate{Method: "GET", URL: "{{base_url}}/health"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: intPtr(200)}}}},
	}
	dir := t.TempDir()
	if _, err := WriteSuite(steps, WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("first WriteSuite: %v", err)
	}
	if _, err := WriteSuite(steps, WriteOptions{Dir: dir, Force: true}); err != nil {
		t.Fatalf("expected force overwrite to succeed, got %v", err)
	}
}

func TestWriteSuite_WrittenFilesCompileViaTestdef(t *testing.T) {
	login := jsonExchange("POST", "http://x/api/users", 201, `{"name":"ada"}`, `{"data":{"id":7}}`)
	fetch := jsonExchange("GET", "http://x/api/users/7", 200, "", `{"data":{"id":7}}`)
	steps, err := Session([]domain.Exchange{login, fetch})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}

	dir := t.TempDir()
	if _, err := WriteSuite(steps, WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("WriteSuite: %v", err)
	}

	tests, err := testdef.NewLoader().LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll on the written suite: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected 2 compiled tests, got %d", len(tests))
	}
	if !tests[1].UsesChaining {
		t.Error("expected the second compiled test to be recognized as using chaining")
	}
}

func TestWriteSuite_MissingDirIsConfigError(t *testing.T) {
	_, err := WriteSuite([]Step{{ID: "a", Test: domain.TestCase{Name: "A"}}}, WriteOptions{})
	if err == nil {
		t.Fatal("expected an error when Dir is empty")
	}
}

func intPtr(i int) *int { return &i }

func TestWriteSuite_CreatesOutputDirIfMissing(t *testing.T) {
	steps := []Step{
		{ID: "a", Test: domain.TestCase{Version: 2, ID: "a", Name: "A",
			Request: domain.RequestTemplate{Method: "GET", URL: "{{base_url}}/health"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: intPtr(200)}}}},
	}
	dir := filepath.Join(t.TempDir(), "nested", "recorded")
	written, err := WriteSuite(steps, WriteOptions{Dir: dir})
	if err != nil {
		t.Fatalf("WriteSuite: %v", err)
	}
	if _, err := os.Stat(written[0]); err != nil {
		t.Errorf("expected written file to exist: %v", err)
	}
}
