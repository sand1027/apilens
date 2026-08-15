package testdef

import "testing"

func TestLoadAll_ValidDirLoadsSortedByPath(t *testing.T) {
	l := NewLoader()
	tests, err := l.LoadAll("../../testdata/tests/valid")
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	// contract-assertions.yaml, full.yaml, minimal.yaml, schema-file.yaml —
	// user.schema.json is skipped (not a .yaml/.yml file).
	if len(tests) != 4 {
		t.Fatalf("expected 4 tests, got %d: %+v", len(tests), tests)
	}
	// contract-assertions.yaml < full.yaml alphabetically
	if tests[0].File[len(tests[0].File)-24:] != "contract-assertions.yaml" {
		t.Errorf("expected sorted order, first file = %s", tests[0].File)
	}
}

func TestLoadAll_InvalidDirReturnsCompileError(t *testing.T) {
	l := NewLoader()
	_, err := l.LoadAll("../../testdata/tests/invalid")
	if err == nil {
		t.Fatal("expected error loading a directory containing invalid tests")
	}
}

func TestLoadAll_MissingDirReturnsEmptyNoError(t *testing.T) {
	l := NewLoader()
	tests, err := l.LoadAll("../../testdata/does-not-exist")
	if err != nil {
		t.Fatalf("LoadAll on missing dir: %v", err)
	}
	if tests != nil {
		t.Errorf("expected nil tests for missing dir, got %v", tests)
	}
}
