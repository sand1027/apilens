package specexport

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestWriteFile_CreatesFileAndParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "export.openapi.yaml")
	doc := Build([]domain.Endpoint{{Method: "GET", Path: "/api/ping"}}, Options{})

	if err := WriteFile(doc, path, false); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected file to exist: %v", err)
	}
}

func TestWriteFile_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.openapi.yaml")
	doc := Build([]domain.Endpoint{{Method: "GET", Path: "/api/ping"}}, Options{})

	if err := WriteFile(doc, path, false); err != nil {
		t.Fatalf("first WriteFile: %v", err)
	}
	err := WriteFile(doc, path, false)
	if err == nil {
		t.Fatal("expected an error overwriting an existing file without --force")
	}
}

func TestWriteFile_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "export.openapi.yaml")
	doc := Build([]domain.Endpoint{{Method: "GET", Path: "/api/ping"}}, Options{})

	if err := WriteFile(doc, path, false); err != nil {
		t.Fatalf("first WriteFile: %v", err)
	}
	if err := WriteFile(doc, path, true); err != nil {
		t.Fatalf("expected force overwrite to succeed, got %v", err)
	}
}
