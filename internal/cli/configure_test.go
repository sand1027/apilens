package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// runConfigure executes `apilens configure <args...>` against a fresh
// project dir (via --project) and returns stdout. stdin is fed for the
// interactive-prompt path.
func runConfigure(t *testing.T, projectDir string, stdin string, args ...string) string {
	t.Helper()
	root := NewRootCommand()
	full := append([]string{"configure", "--project", projectDir}, args...)
	root.SetArgs(full)
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(stdin))
	if err := root.Execute(); err != nil {
		t.Fatalf("apilens configure %v: %v\noutput:\n%s", args, err, out.String())
	}
	return out.String()
}

func TestConfigure_NonInteractiveWritesTokenAndDBConnection(t *testing.T) {
	dir := t.TempDir()
	runConfigure(t, dir, "", "--token", "tok-123", "--db-driver", "sqlite", "--db-dsn", "file:./app.db")

	secretsEnv, err := os.ReadFile(filepath.Join(dir, ".apilens", ".secrets.env"))
	if err != nil {
		t.Fatalf("reading .secrets.env: %v", err)
	}
	if !strings.Contains(string(secretsEnv), "AUTH_TOKEN=tok-123") {
		t.Errorf(".secrets.env missing AUTH_TOKEN: %s", secretsEnv)
	}
	if !strings.Contains(string(secretsEnv), "DATABASE_URL=file:./app.db") {
		t.Errorf(".secrets.env missing DATABASE_URL: %s", secretsEnv)
	}

	envSecrets, err := os.ReadFile(filepath.Join(dir, ".apilens", "environments", "local.secrets.yaml"))
	if err != nil {
		t.Fatalf("reading local.secrets.yaml: %v", err)
	}
	if !strings.Contains(string(envSecrets), "tok-123") {
		t.Errorf("local.secrets.yaml missing token: %s", envSecrets)
	}

	configYAML, err := os.ReadFile(filepath.Join(dir, ".apilens", "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	if !strings.Contains(string(configYAML), "${DATABASE_URL}") {
		t.Errorf("config.yaml should reference ${DATABASE_URL}, not a literal DSN: %s", configYAML)
	}
	if strings.Contains(string(configYAML), "file:./app.db") {
		t.Error("config.yaml must never contain the literal DSN")
	}
}

func TestConfigure_MissingDSNIsAnError(t *testing.T) {
	dir := t.TempDir()
	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--db-driver", "postgres"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when --db-driver is set without --db-dsn")
	}
}

func TestConfigure_UnsupportedDriverIsAnError(t *testing.T) {
	dir := t.TempDir()
	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--db-driver", "oracle", "--db-dsn", "oracle://x"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an unsupported db driver")
	}
}

func TestConfigure_MongoDriverWithoutDatabaseNameInDSNIsAnError(t *testing.T) {
	dir := t.TempDir()
	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--db-driver", "mongodb", "--db-dsn", "mongodb://localhost:27017"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when the MongoDB DSN has no database name")
	}
}

func TestConfigure_MongoDriverWritesConnection(t *testing.T) {
	dir := t.TempDir()
	runConfigure(t, dir, "", "--db-driver", "mongodb", "--db-dsn", "mongodb://localhost:27017/mydb")

	configYAML, err := os.ReadFile(filepath.Join(dir, ".apilens", "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	if !strings.Contains(string(configYAML), "driver: mongodb") {
		t.Errorf("config.yaml missing mongodb driver: %s", configYAML)
	}
	if !strings.Contains(string(configYAML), "${DATABASE_URL}") {
		t.Errorf("config.yaml should reference ${DATABASE_URL}: %s", configYAML)
	}

	secretsEnv, err := os.ReadFile(filepath.Join(dir, ".apilens", ".secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secretsEnv), "DATABASE_URL=mongodb://localhost:27017/mydb") {
		t.Errorf(".secrets.env missing DATABASE_URL: %s", secretsEnv)
	}
}

func TestConfigure_TokenOnlyDoesNotTouchDBConfig(t *testing.T) {
	dir := t.TempDir()
	runConfigure(t, dir, "", "--token", "only-token")
	if _, err := os.Stat(filepath.Join(dir, ".apilens", "config.yaml")); err == nil {
		t.Error("config.yaml should not be created when only a token is configured")
	}
}

func TestConfigure_InteractivePromptsNeverAskForToken(t *testing.T) {
	// The token is never part of the interactive flow (a real token, e.g.
	// a JWT, routinely exceeds a terminal's canonical-mode line-input
	// buffer, which makes prompting for it hang with no way to recover
	// except Ctrl+C -- see newConfigureCommand's doc comment). Only
	// env/db-driver/db-dsn are prompted for, in that order.
	dir := t.TempDir()
	stdin := "staging\nsqlite\nfile:./staging.db\n"
	out := runConfigure(t, dir, stdin)

	// The prompt text itself (not paths in the "Updated:" summary, which
	// can coincidentally contain "Token" via the test's own temp dir
	// name) must never ask about a token.
	firstLine := strings.SplitN(out, "\n", 2)[0]
	if strings.Contains(strings.ToLower(firstLine), "token") {
		t.Errorf("interactive prompt output must never mention the token, got:\n%s", out)
	}
	if !strings.HasPrefix(out, "Environment [") {
		t.Errorf("expected the interactive flow to start by asking for the environment, got:\n%s", out)
	}

	// No token should have been written anywhere -- nothing in stdin
	// beyond env/driver/dsn was ever asked for.
	if _, err := os.Stat(filepath.Join(dir, ".apilens", "environments", "staging.secrets.yaml")); err == nil {
		t.Error("no token was provided, so no <env>.secrets.yaml should have been written")
	}

	configYAML, err := os.ReadFile(filepath.Join(dir, ".apilens", "config.yaml"))
	if err != nil {
		t.Fatalf("reading config.yaml: %v", err)
	}
	if !strings.Contains(string(configYAML), "driver: sqlite") {
		t.Errorf("config.yaml missing sqlite driver: %s", configYAML)
	}
}

func TestConfigure_TokenFallsBackToAuthTokenEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUTH_TOKEN", "from-env-var")

	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--yes"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetIn(strings.NewReader(""))
	if err := root.Execute(); err != nil {
		t.Fatalf("apilens configure: %v\noutput:\n%s", err, out.String())
	}

	envSecrets, err := os.ReadFile(filepath.Join(dir, ".apilens", "environments", "local.secrets.yaml"))
	if err != nil {
		t.Fatalf("reading local.secrets.yaml: %v", err)
	}
	if !strings.Contains(string(envSecrets), "from-env-var") {
		t.Errorf("expected the AUTH_TOKEN env var to be picked up, got %s", envSecrets)
	}
}

func TestConfigure_ExplicitTokenFlagWinsOverEnvVar(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AUTH_TOKEN", "from-env-var")

	runConfigure(t, dir, "", "--token", "from-flag", "--yes")

	envSecrets, err := os.ReadFile(filepath.Join(dir, ".apilens", "environments", "local.secrets.yaml"))
	if err != nil {
		t.Fatalf("reading local.secrets.yaml: %v", err)
	}
	if !strings.Contains(string(envSecrets), "from-flag") {
		t.Errorf("expected --token to win over $AUTH_TOKEN, got %s", envSecrets)
	}
	if strings.Contains(string(envSecrets), "from-env-var") {
		t.Errorf("did not expect the env var value to be written, got %s", envSecrets)
	}
}

func TestConfigure_DBDSNFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFilePath := filepath.Join(dir, "app.env")
	envFileContent := "PORT=3000\nMONGO_CONNECTION_URL=\"mongodb+srv://user:pass@cluster.mongodb.net/mydb?retryWrites=true\"\n"
	if err := os.WriteFile(envFilePath, []byte(envFileContent), 0o644); err != nil {
		t.Fatal(err)
	}

	runConfigure(t, dir, "", "--db-driver", "mongodb",
		"--db-dsn-from-env-file", envFilePath, "--db-dsn-key", "MONGO_CONNECTION_URL")

	secretsEnv, err := os.ReadFile(filepath.Join(dir, ".apilens", ".secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secretsEnv), "DATABASE_URL=mongodb+srv://user:pass@cluster.mongodb.net/mydb?retryWrites=true") {
		t.Errorf(".secrets.env missing the DSN read from the env file, got %s", secretsEnv)
	}

	configYAML, err := os.ReadFile(filepath.Join(dir, ".apilens", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(configYAML), "cluster.mongodb.net") {
		t.Error("config.yaml must never contain the literal DSN, even when sourced from an env file")
	}
}

func TestConfigure_DBDSNFromEnvFile_MissingKeyIsAnError(t *testing.T) {
	dir := t.TempDir()
	envFilePath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(envFilePath, []byte("PORT=3000\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--db-driver", "mongodb",
		"--db-dsn-from-env-file", envFilePath, "--db-dsn-key", "MONGO_CONNECTION_URL"})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when the requested key is missing from the env file")
	}
}

func TestConfigure_DBDSNFromEnvFile_MissingFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	root := NewRootCommand()
	root.SetArgs([]string{"configure", "--project", dir, "--db-driver", "mongodb",
		"--db-dsn-from-env-file", filepath.Join(dir, "does-not-exist.env")})
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error when the env file does not exist")
	}
}

func TestConfigure_TokenFromEnvFile(t *testing.T) {
	dir := t.TempDir()
	envFilePath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(envFilePath, []byte("AUTH_TOKEN=tok-from-file\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runConfigure(t, dir, "", "--token-from-env-file", envFilePath)

	envSecrets, err := os.ReadFile(filepath.Join(dir, ".apilens", "environments", "local.secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envSecrets), "tok-from-file") {
		t.Errorf("expected the token read from the env file, got %s", envSecrets)
	}
}

func TestConfigure_TokenFromEnvFile_CustomKey(t *testing.T) {
	dir := t.TempDir()
	envFilePath := filepath.Join(dir, "app.env")
	if err := os.WriteFile(envFilePath, []byte("AUTH_SECRET=tok-custom-key\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	runConfigure(t, dir, "", "--token-from-env-file", envFilePath, "--token-env-key", "AUTH_SECRET")

	envSecrets, err := os.ReadFile(filepath.Join(dir, ".apilens", "environments", "local.secrets.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envSecrets), "tok-custom-key") {
		t.Errorf("expected the token read from the custom key, got %s", envSecrets)
	}
}

func TestConfigure_RunningTwiceMergesRatherThanClobbers(t *testing.T) {
	dir := t.TempDir()
	runConfigure(t, dir, "", "--token", "first-token")
	runConfigure(t, dir, "", "--db-driver", "sqlite", "--db-dsn", "file:./app.db")

	secretsEnv, err := os.ReadFile(filepath.Join(dir, ".apilens", ".secrets.env"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(secretsEnv), "AUTH_TOKEN=first-token") {
		t.Errorf("expected the earlier AUTH_TOKEN to survive a later configure call, got %s", secretsEnv)
	}
	if !strings.Contains(string(secretsEnv), "DATABASE_URL=file:./app.db") {
		t.Errorf("expected DATABASE_URL to be added, got %s", secretsEnv)
	}
}
