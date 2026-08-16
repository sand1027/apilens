package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetDBConnection_WritesConnectionIntoExistingConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(configTemplateForTest), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := SetDBConnection(path, "main", "sqlite", "DATABASE_URL"); err != nil {
		t.Fatalf("SetDBConnection: %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	conn, ok := cfg.DB.Connections["main"]
	if !ok {
		t.Fatal("expected db.connections.main to be set")
	}
	if conn.Driver != "sqlite" {
		t.Errorf("Driver = %q, want sqlite", conn.Driver)
	}
	if conn.DSN != "${DATABASE_URL}" {
		t.Errorf("DSN = %q, want the env placeholder, not a literal secret", conn.DSN)
	}

	// The rest of the file must survive untouched.
	raw, _ := os.ReadFile(path)
	if !strings.Contains(string(raw), "base_url: \"http://example.test\"") {
		t.Errorf("existing config content was clobbered:\n%s", raw)
	}
}

func TestSetDBConnection_CreatesFileWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "config.yaml")

	if err := SetDBConnection(path, "main", "postgres", "DATABASE_URL"); err != nil {
		t.Fatalf("SetDBConnection: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.DB.Connections["main"].Driver != "postgres" {
		t.Errorf("driver = %q", cfg.DB.Connections["main"].Driver)
	}
}

func TestSetDBConnection_SecondCallOverwritesSameConnectionName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := SetDBConnection(path, "main", "sqlite", "OLD_VAR"); err != nil {
		t.Fatal(err)
	}
	if err := SetDBConnection(path, "main", "postgres", "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DB.Connections) != 1 {
		t.Fatalf("expected exactly one connection, got %v", cfg.DB.Connections)
	}
	conn := cfg.DB.Connections["main"]
	if conn.Driver != "postgres" || conn.DSN != "${DATABASE_URL}" {
		t.Errorf("connection not updated in place: %+v", conn)
	}
}

func TestSetDBConnection_AddingSecondConnectionKeepsFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := SetDBConnection(path, "main", "sqlite", "DATABASE_URL"); err != nil {
		t.Fatal(err)
	}
	if err := SetDBConnection(path, "analytics", "postgres", "ANALYTICS_DSN"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DB.Connections) != 2 {
		t.Fatalf("expected two connections, got %v", cfg.DB.Connections)
	}
	if cfg.DB.Connections["main"].Driver != "sqlite" {
		t.Error("first connection was lost")
	}
	if cfg.DB.Connections["analytics"].Driver != "postgres" {
		t.Error("second connection was not added")
	}
}

func TestValidateDBDriver(t *testing.T) {
	for _, ok := range []string{"sqlite", "sqlite3", "postgres", "postgresql", "pgx", "SQLite", "mongodb", "mongo", "MongoDB"} {
		if err := ValidateDBDriver(ok); err != nil {
			t.Errorf("expected %q to be valid, got %v", ok, err)
		}
	}
	for _, bad := range []string{"oracle", ""} {
		if err := ValidateDBDriver(bad); err == nil {
			t.Errorf("expected %q to be rejected", bad)
		}
	}
}

const configTemplateForTest = `server:
  base_url: "http://example.test"

testing:
  parallel: true
`
