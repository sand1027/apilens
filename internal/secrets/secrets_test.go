package secrets

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSet_WritesKeyValuePairs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.env")

	if err := Set(path, map[string]string{"AUTH_TOKEN": "abc123"}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading written file: %v", err)
	}
	if !contains(string(raw), "AUTH_TOKEN=abc123") {
		t.Errorf("file content = %q", raw)
	}
}

func TestSet_MergesWithoutClobberingExistingKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.env")

	if err := Set(path, map[string]string{"AUTH_TOKEN": "abc123"}); err != nil {
		t.Fatalf("first Set: %v", err)
	}
	if err := Set(path, map[string]string{"DATABASE_URL": "file:./app.db"}); err != nil {
		t.Fatalf("second Set: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	if !contains(content, "AUTH_TOKEN=abc123") {
		t.Errorf("expected AUTH_TOKEN to survive a later Set call, got %q", content)
	}
	if !contains(content, "DATABASE_URL=file:./app.db") {
		t.Errorf("expected DATABASE_URL to be written, got %q", content)
	}
}

func TestSet_OverwritesSameKey(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.env")

	if err := Set(path, map[string]string{"AUTH_TOKEN": "old"}); err != nil {
		t.Fatal(err)
	}
	if err := Set(path, map[string]string{"AUTH_TOKEN": "new"}); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	content := string(raw)
	if contains(content, "AUTH_TOKEN=old") {
		t.Error("old value should have been replaced")
	}
	if !contains(content, "AUTH_TOKEN=new") {
		t.Errorf("expected new value, got %q", content)
	}
}

func TestLoad_SetsUnsetEnvVars(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.env")
	if err := Set(path, map[string]string{"APILENS_TEST_SECRET_A": "from-file"}); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("APILENS_TEST_SECRET_A")
	t.Cleanup(func() { os.Unsetenv("APILENS_TEST_SECRET_A") })

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("APILENS_TEST_SECRET_A"); got != "from-file" {
		t.Errorf("got %q, want %q", got, "from-file")
	}
}

func TestLoad_DoesNotOverrideAlreadySetEnvVar(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".secrets.env")
	if err := Set(path, map[string]string{"APILENS_TEST_SECRET_B": "from-file"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("APILENS_TEST_SECRET_B", "from-shell")

	if err := Load(path); err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := os.Getenv("APILENS_TEST_SECRET_B"); got != "from-shell" {
		t.Errorf("an explicitly exported env var must win, got %q", got)
	}
}

func TestLoad_MissingFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	if err := Load(filepath.Join(dir, "does-not-exist.env")); err != nil {
		t.Errorf("a missing secrets file should not be an error, got %v", err)
	}
}

func TestReadEnvFile_ParsesKeyValuePairs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.env")
	content := "PORT=3000\n# a comment\nMONGO_CONNECTION_URL=\"mongodb://host/db\"\n\nAUTH_TOKEN=abc\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	kv, err := ReadEnvFile(path)
	if err != nil {
		t.Fatalf("ReadEnvFile: %v", err)
	}
	if kv["PORT"] != "3000" {
		t.Errorf("PORT = %q", kv["PORT"])
	}
	if kv["MONGO_CONNECTION_URL"] != "mongodb://host/db" {
		t.Errorf("MONGO_CONNECTION_URL = %q (quotes should be stripped)", kv["MONGO_CONNECTION_URL"])
	}
	if kv["AUTH_TOKEN"] != "abc" {
		t.Errorf("AUTH_TOKEN = %q", kv["AUTH_TOKEN"])
	}
	if _, ok := kv["a comment"]; ok {
		t.Error("comment line should not be parsed as a key")
	}
}

func TestReadEnvFile_DoesNotTouchProcessEnvironment(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "app.env")
	if err := os.WriteFile(path, []byte("APILENS_TEST_READENVFILE_ISOLATION=set-by-file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	os.Unsetenv("APILENS_TEST_READENVFILE_ISOLATION")
	t.Cleanup(func() { os.Unsetenv("APILENS_TEST_READENVFILE_ISOLATION") })

	if _, err := ReadEnvFile(path); err != nil {
		t.Fatal(err)
	}
	if v := os.Getenv("APILENS_TEST_READENVFILE_ISOLATION"); v != "" {
		t.Errorf("ReadEnvFile must not mutate the process environment, got %q", v)
	}
}

func TestPath_JoinsProjectDirAndFilename(t *testing.T) {
	got := Path("/tmp/myproject")
	want := filepath.Join("/tmp/myproject", ".apilens", ".secrets.env")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
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
