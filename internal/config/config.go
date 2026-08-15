// Package config loads and validates ".apilens/config.yaml". See
// docs/09-security.md section 10 and docs/03-plugins.md section 9 for the
// documented defaults this schema implements.
package config

import (
	"os"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// Config is the root config.yaml schema. Discovery lands in v2
// (docs/02-packages.md, docs/03-plugins.md section 9); watch is still
// deferred to v3.
type Config struct {
	Server    ServerConfig    `yaml:"server"`
	Testing   TestingConfig   `yaml:"testing"`
	Security  SecurityConfig  `yaml:"security"`
	Discovery DiscoveryConfig `yaml:"discovery"`
	Watch     WatchConfig     `yaml:"watch"`
}

type ServerConfig struct {
	BaseURL string `yaml:"base_url"`
}

type TestingConfig struct {
	Parallel bool          `yaml:"parallel"`
	Workers  int           `yaml:"workers"`
	Retries  int           `yaml:"retries"`
	Timeout  time.Duration `yaml:"timeout"`
	Reporter string        `yaml:"reporter"`
}

type SecurityConfig struct {
	CaptureSensitiveHeaders bool     `yaml:"capture_sensitive_headers"`
	SensitiveHeaders        []string `yaml:"sensitive_headers"`
	MaxResponseSize         string   `yaml:"max_response_size"`
	ProxyAllowRemote        bool     `yaml:"proxy_allow_remote"`
}

// DiscoveryConfig mirrors docs/03-plugins.md section 9's documented
// `discovery:` block. openapi is enabled by default; express is opt-in
// until its heuristics have proven themselves on a given project
// (docs/03-plugins.md section 9: "express: enabled: false # Phase 2").
type DiscoveryConfig struct {
	OpenAPI OpenAPIDiscoveryConfig `yaml:"openapi"`
	Express ExpressDiscoveryConfig `yaml:"express"`
}

type OpenAPIDiscoveryConfig struct {
	Enabled bool     `yaml:"enabled"`
	Paths   []string `yaml:"paths"`
}

type ExpressDiscoveryConfig struct {
	Enabled bool `yaml:"enabled"`
}

// WatchConfig mirrors docs/08-proxy.md section 6's documented `watch:`
// block. Bind/Port are also settable via CLI flags (docs/05-cli.md
// "apilens watch"); the CLI flag wins when set.
type WatchConfig struct {
	Bind             string   `yaml:"bind"`
	Port             int      `yaml:"port"`
	PathPrefix       string   `yaml:"path_prefix"`
	IgnoreExtensions []string `yaml:"ignore_extensions"`
}

// rawTestingConfig lets us accept a human duration string ("10s") for
// testing.timeout while keeping the public Config.Testing.Timeout typed as
// time.Duration.
type rawConfig struct {
	Server  ServerConfig `yaml:"server"`
	Testing struct {
		Parallel bool   `yaml:"parallel"`
		Workers  int    `yaml:"workers"`
		Retries  int    `yaml:"retries"`
		Timeout  string `yaml:"timeout"`
		Reporter string `yaml:"reporter"`
	} `yaml:"testing"`
	Security SecurityConfig `yaml:"security"`
	// Discovery uses pointer fields so Load can tell "the user wrote
	// discovery.openapi.enabled: false" apart from "the user didn't
	// mention discovery at all" — otherwise a zero-value bool would
	// silently disable the openapi provider's true-by-default setting.
	Discovery struct {
		OpenAPI struct {
			Enabled *bool    `yaml:"enabled"`
			Paths   []string `yaml:"paths"`
		} `yaml:"openapi"`
		Express struct {
			Enabled *bool `yaml:"enabled"`
		} `yaml:"express"`
	} `yaml:"discovery"`
	Watch WatchConfig `yaml:"watch"`
}

// Default returns the built-in defaults, per docs/09-security.md section 10
// and docs/11-risks-and-gaps.md G13/G16.
func Default() Config {
	return Config{
		Testing: TestingConfig{
			Parallel: true,
			Retries:  1,
			Timeout:  10 * time.Second,
			Reporter: "terminal",
		},
		Security: SecurityConfig{
			CaptureSensitiveHeaders: false,
			MaxResponseSize:         "5MB",
			ProxyAllowRemote:        false,
		},
		Discovery: DiscoveryConfig{
			OpenAPI: OpenAPIDiscoveryConfig{Enabled: true},
			Express: ExpressDiscoveryConfig{Enabled: false},
		},
		Watch: WatchConfig{
			Bind:             "127.0.0.1",
			Port:             8888,
			IgnoreExtensions: append([]string(nil), defaultIgnoreExtensions...),
		},
	}
}

// defaultIgnoreExtensions mirrors proxy.DefaultIgnoreExtensions. Duplicated
// as a plain slice literal here (rather than importing internal/proxy) to
// avoid a config -> proxy dependency; internal/proxy already depends on
// nothing from config, keeping the dependency direction one-way
// (docs/02-packages.md section 2).
var defaultIgnoreExtensions = []string{"js", "css", "map", "png", "jpg", "svg", "woff2"}

// Load reads path, merging over Default(). A missing file is not an error —
// callers get Default() so `apilens run` before `init` still has sane
// values (though app.RunSuite separately requires .apilens/tests to exist).
func Load(path string) (Config, error) {
	cfg := Default()
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, domain.NewConfigError("reading config file "+path, err)
	}

	var rc rawConfig
	if err := yaml.Unmarshal(raw, &rc); err != nil {
		return cfg, domain.NewConfigError("parsing config file "+path, err)
	}

	if rc.Server.BaseURL != "" {
		cfg.Server.BaseURL = rc.Server.BaseURL
	}
	// testing.* — only overlay fields the user actually set. Zero-value
	// bool "parallel: false" is indistinguishable from "unset" in this
	// simple decode; that's acceptable for v1 (docs/11-risks-and-gaps.md
	// G13 defers full merge semantics; CLI flags are the override path).
	if rc.Testing.Reporter != "" {
		cfg.Testing.Reporter = rc.Testing.Reporter
	}
	if rc.Testing.Timeout != "" {
		d, err := time.ParseDuration(rc.Testing.Timeout)
		if err != nil {
			return cfg, domain.NewConfigError("invalid testing.timeout "+rc.Testing.Timeout, err)
		}
		cfg.Testing.Timeout = d
	}
	if rc.Testing.Workers > 0 {
		cfg.Testing.Workers = rc.Testing.Workers
	}
	if rc.Testing.Retries != 0 {
		cfg.Testing.Retries = rc.Testing.Retries
	}
	cfg.Testing.Parallel = rc.Testing.Parallel || cfg.Testing.Parallel

	if len(rc.Security.SensitiveHeaders) > 0 {
		cfg.Security.SensitiveHeaders = rc.Security.SensitiveHeaders
	}
	if rc.Security.MaxResponseSize != "" {
		cfg.Security.MaxResponseSize = rc.Security.MaxResponseSize
	}
	cfg.Security.CaptureSensitiveHeaders = rc.Security.CaptureSensitiveHeaders
	cfg.Security.ProxyAllowRemote = rc.Security.ProxyAllowRemote

	if rc.Discovery.OpenAPI.Enabled != nil {
		cfg.Discovery.OpenAPI.Enabled = *rc.Discovery.OpenAPI.Enabled
	}
	if len(rc.Discovery.OpenAPI.Paths) > 0 {
		cfg.Discovery.OpenAPI.Paths = rc.Discovery.OpenAPI.Paths
	}
	if rc.Discovery.Express.Enabled != nil {
		cfg.Discovery.Express.Enabled = *rc.Discovery.Express.Enabled
	}

	if rc.Watch.Bind != "" {
		cfg.Watch.Bind = rc.Watch.Bind
	}
	if rc.Watch.Port != 0 {
		cfg.Watch.Port = rc.Watch.Port
	}
	if rc.Watch.PathPrefix != "" {
		cfg.Watch.PathPrefix = rc.Watch.PathPrefix
	}
	if len(rc.Watch.IgnoreExtensions) > 0 {
		cfg.Watch.IgnoreExtensions = rc.Watch.IgnoreExtensions
	}

	return cfg, nil
}

// MaxResponseSizeBytes parses "5MB" / "1048576" style values into bytes.
// Falls back to security.DefaultMaxResponseSize on empty/unparseable input.
func (c Config) MaxResponseSizeBytes() int64 {
	return parseSize(c.Security.MaxResponseSize)
}
