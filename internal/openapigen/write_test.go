package openapigen

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/testdef"
)

func statusPtr(i int) *int { return &i }

func sampleGenerated() []Generated {
	return []Generated{
		{OperationID: "health", Test: domain.TestCase{
			Version: 1, Name: "health",
			Request: domain.RequestTemplate{Method: "GET", URL: "{{base_url}}/health"},
			Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: statusPtr(200)}},
		}},
		{OperationID: "createUser", Test: domain.TestCase{
			Version: 1, Name: "createUser",
			Request: domain.RequestTemplate{
				Method: "POST", URL: "{{base_url}}/api/users",
				Body: domain.BodyTemplate{JSON: map[string]any{"name": "Ada"}},
			},
			Assert: domain.AssertionSpec{Status: &domain.StatusSpec{Equals: statusPtr(201)}},
		}},
	}
}

func TestWriteAll_WritesOneFilePerGeneratedTest(t *testing.T) {
	dir := t.TempDir()
	written, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir})
	if err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	if len(written) != 2 {
		t.Fatalf("expected 2 files, got %d: %v", len(written), written)
	}
	for _, f := range written {
		if _, err := os.Stat(f); err != nil {
			t.Errorf("expected %s to exist: %v", f, err)
		}
	}
}

func TestWriteAll_FilenamesFollowMethodPathConvention(t *testing.T) {
	dir := t.TempDir()
	written, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir})
	if err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	names := map[string]bool{}
	for _, f := range written {
		names[filepath.Base(f)] = true
	}
	if !names["get-health.yaml"] {
		t.Errorf("expected get-health.yaml, got %v", names)
	}
	if !names["post-api-users.yaml"] {
		t.Errorf("expected post-api-users.yaml, got %v", names)
	}
}

func TestWriteAll_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("first WriteAll: %v", err)
	}
	if _, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir}); err == nil {
		t.Fatal("expected an error overwriting existing files without --force")
	}
}

func TestWriteAll_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("first WriteAll: %v", err)
	}
	if _, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir, Force: true}); err != nil {
		t.Fatalf("expected force overwrite to succeed, got %v", err)
	}
}

func TestWriteAll_MissingDirIsConfigError(t *testing.T) {
	_, err := WriteAll(sampleGenerated(), WriteOptions{})
	if err == nil {
		t.Fatal("expected an error when Dir is empty")
	}
}

func TestWriteAll_WrittenFilesCompileViaTestdef(t *testing.T) {
	dir := t.TempDir()
	if _, err := WriteAll(sampleGenerated(), WriteOptions{Dir: dir}); err != nil {
		t.Fatalf("WriteAll: %v", err)
	}
	tests, err := testdef.NewLoader().LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll on the written tests: %v", err)
	}
	if len(tests) != 2 {
		t.Fatalf("expected 2 compiled tests, got %d", len(tests))
	}
}
