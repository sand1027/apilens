package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad_MissingFileReturnsDefaults(t *testing.T) {
	cfg, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	def := Default()
	if cfg.Testing.Timeout != def.Testing.Timeout {
		t.Errorf("Timeout = %v, want default %v", cfg.Testing.Timeout, def.Testing.Timeout)
	}
	if cfg.Testing.Reporter != "terminal" {
		t.Errorf("Reporter = %q, want terminal", cfg.Testing.Reporter)
	}
	if !cfg.Discovery.GraphQL.Enabled {
		t.Error("expected discovery.graphql.enabled true by default")
	}
}

func TestLoad_OverlaysUserValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
server:
  base_url: "https://example.com"
testing:
  reporter: json
  timeout: 30s
  workers: 4
security:
  max_response_size: 1MB
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Server.BaseURL != "https://example.com" {
		t.Errorf("BaseURL = %q", cfg.Server.BaseURL)
	}
	if cfg.Testing.Reporter != "json" {
		t.Errorf("Reporter = %q", cfg.Testing.Reporter)
	}
	if cfg.Testing.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v", cfg.Testing.Timeout)
	}
	if cfg.Testing.Workers != 4 {
		t.Errorf("Workers = %d", cfg.Testing.Workers)
	}
	if cfg.MaxResponseSizeBytes() != 1024*1024 {
		t.Errorf("MaxResponseSizeBytes = %d, want %d", cfg.MaxResponseSizeBytes(), 1024*1024)
	}
}

func TestLoad_InvalidTimeoutReturnsConfigError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("testing:\n  timeout: not-a-duration\n"), 0o644); err != nil {
		t.Fatalf("writing config: %v", err)
	}
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestParseSize(t *testing.T) {
	const defaultFallback = 5 * 1024 * 1024 // security.DefaultMaxResponseSize
	cases := map[string]int64{
		"5MB":   5 * 1024 * 1024,
		"512KB": 512 * 1024,
		"1GB":   1024 * 1024 * 1024,
		"100":   100,
		"":      defaultFallback,
		"bogus": defaultFallback,
	}
	for in, want := range cases {
		if got := parseSize(in); got != want {
			t.Errorf("parseSize(%q) = %d, want %d", in, got, want)
		}
	}
}
