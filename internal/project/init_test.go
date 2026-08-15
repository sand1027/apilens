package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInit_CreatesExpectedLayout(t *testing.T) {
	dir := t.TempDir()
	res, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if len(res.Created) == 0 {
		t.Fatal("expected files to be created")
	}

	expect := []string{
		".apilens/config.yaml",
		".apilens/tests/.gitkeep",
		".apilens/environments/local.yaml",
		".apilens/api/.gitkeep",
		".apilens/tests/smoke/health.yaml",
	}
	for _, rel := range expect {
		path := filepath.Join(dir, rel)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}

	gitignore, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("reading .gitignore: %v", err)
	}
	if !contains(string(gitignore), ".apilens/reports/") {
		t.Error(".gitignore missing ApiLens snippet")
	}
}

func TestInit_SecondRunSkipsExistingFilesWithoutForce(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir, false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	// Modify config.yaml to prove it's not clobbered.
	configPath := filepath.Join(dir, ".apilens", "config.yaml")
	if err := os.WriteFile(configPath, []byte("# user edited\n"), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}

	res, err := Init(dir, false)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	found := false
	for _, s := range res.Skipped {
		if s == configPath {
			found = true
		}
	}
	if !found {
		t.Errorf("expected config.yaml to be skipped, got Skipped=%v", res.Skipped)
	}

	data, _ := os.ReadFile(configPath)
	if string(data) != "# user edited\n" {
		t.Error("config.yaml was overwritten without --force")
	}
}

func TestInit_ForceOverwritesConfigOnly(t *testing.T) {
	dir := t.TempDir()
	if _, err := Init(dir, false); err != nil {
		t.Fatalf("first Init: %v", err)
	}
	configPath := filepath.Join(dir, ".apilens", "config.yaml")
	if err := os.WriteFile(configPath, []byte("# user edited\n"), 0o644); err != nil {
		t.Fatalf("writing test config: %v", err)
	}
	// Also edit the sample test to prove force does NOT touch it.
	testPath := filepath.Join(dir, ".apilens", "tests", "smoke", "health.yaml")
	if err := os.WriteFile(testPath, []byte("# user edited test\n"), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}

	if _, err := Init(dir, true); err != nil {
		t.Fatalf("forced Init: %v", err)
	}

	data, _ := os.ReadFile(configPath)
	if string(data) == "# user edited\n" {
		t.Error("expected config.yaml to be overwritten with --force")
	}

	testData, _ := os.ReadFile(testPath)
	if string(testData) != "# user edited test\n" {
		t.Error("--force should not touch the user's edited sample test")
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
