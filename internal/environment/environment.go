// Package environment loads `.apilens/environments/<name>.yaml` files and
// interpolates "{{var}}" / "${ENV}" placeholders per docs/06-test-dsl.md
// section 7 and docs/04-interfaces.md section 5.
package environment

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/auth"
	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// fileDocument mirrors the on-disk YAML shape of an environment file.
type fileDocument struct {
	BaseURL   string            `yaml:"base_url"`
	Variables map[string]string `yaml:"variables"`
}

var (
	varPattern = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_.]+)\s*\}\}`)
	envPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)
)

// Resolver implements docs/04-interfaces.md section 5. It holds the loaded
// environments and tracks which one is "current".
type Resolver struct {
	envs    map[domain.EnvName]domain.Environment
	current domain.EnvName
	osEnv   func(string) (string, bool)
	auth    *auth.Registry
}

// New builds an empty Resolver with the default auth schemes registered.
// Use LoadDir to populate environments.
func New() *Resolver {
	return &Resolver{
		envs: make(map[domain.EnvName]domain.Environment),
		osEnv: func(key string) (string, bool) {
			return os.LookupEnv(key)
		},
		auth: auth.NewDefaultRegistry(),
	}
}

// LoadDir reads every "*.yaml" / "*.yml" file in dir as an environment,
// naming each by its filename stem. `${ENV}` placeholders inside
// `variables` are NOT expanded here — see loadFile's comment for why.
func (r *Resolver) LoadDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // no environments directory yet is not fatal at load time
		}
		return domain.NewConfigError("reading environments directory", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			if strings.Contains(name, ".secrets.") {
				continue // never auto-load secrets files (docs/09-security.md section 7)
			}
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		path := dir + "/" + name
		if err := r.loadFile(path, envNameFromFilename(name)); err != nil {
			return err
		}
	}
	return nil
}

func envNameFromFilename(name string) domain.EnvName {
	stem := strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
	return domain.EnvName(stem)
}

// loadFile reads an environment file. "${ENV}" placeholders inside
// `variables` are deliberately NOT expanded here — expansion (and the
// fail-closed check for a missing OS variable) happens lazily in
// Interpolate, only for variables a command actually uses. Loading an
// environment must not require every secret it might ever reference to
// already be set: `apilens discover`, `list`, `inspect` (without --live),
// and `env list` never interpolate anything, so they must not fail just
// because AUTH_TOKEN isn't exported. ADR-015's "fail closed" still holds —
// it now fires at the point of use rather than at load time.
func (r *Resolver) loadFile(path string, name domain.EnvName) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.NewConfigError(fmt.Sprintf("reading environment file %s", path), err)
	}
	var doc fileDocument
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return domain.NewConfigError(fmt.Sprintf("parsing environment file %s", path), err)
	}
	vars := make(map[string]string, len(doc.Variables))
	for k, v := range doc.Variables {
		vars[k] = v // raw; may still contain "${ENV}" — expanded lazily
	}
	// Step 2 of docs/06-test-dsl.md section 7: OS env overlays file
	// values for the same variable key. This is a plain key-name lookup
	// (not "${ENV}" expansion), so it cannot fail and stays eager.
	for k := range vars {
		if v, ok := r.osEnv(k); ok {
			vars[k] = v
		}
	}
	baseURL := doc.BaseURL
	if v, ok := r.osEnv("APILENS_BASE_URL"); ok && v != "" {
		baseURL = v
	}
	r.envs[name] = domain.Environment{
		Name:      name,
		BaseURL:   baseURL,
		Variables: vars,
	}
	return nil
}

// expandOSEnv expands every "${NAME}" in s from the process environment.
// A reference to an unset variable is a config error (fail closed).
func (r *Resolver) expandOSEnv(s string) (string, error) {
	var missing []string
	result := envPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := envPattern.FindStringSubmatch(match)[1]
		if v, ok := r.osEnv(name); ok {
			return v
		}
		missing = append(missing, name)
		return match
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required environment variable(s): %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// Use selects the current environment by name.
func (r *Resolver) Use(name domain.EnvName) error {
	if _, ok := r.envs[name]; !ok {
		return domain.NewNotFoundError(fmt.Sprintf("unknown environment %q", name))
	}
	r.current = name
	return nil
}

// Current returns the currently selected environment, or a zero value if
// none has been selected/loaded.
func (r *Resolver) Current() domain.Environment {
	return r.envs[r.current]
}

// CurrentName returns the name of the currently selected environment.
func (r *Resolver) CurrentName() domain.EnvName {
	return r.current
}

// List returns every loaded environment, sorted by name.
func (r *Resolver) List() []domain.Environment {
	names := make([]string, 0, len(r.envs))
	for n := range r.envs {
		names = append(names, string(n))
	}
	sort.Strings(names)
	out := make([]domain.Environment, 0, len(names))
	for _, n := range names {
		out = append(out, r.envs[domain.EnvName(n)])
	}
	return out
}

// Get returns a specific environment by name.
func (r *Resolver) Get(name domain.EnvName) (domain.Environment, bool) {
	e, ok := r.envs[name]
	return e, ok
}

// Interpolate expands every "{{name}}" in s using the current environment's
// variables plus the built-in "base_url". A variable's own value may still
// contain "${ENV}" (deferred by loadFile); that is expanded here, lazily,
// against the process environment. An undefined "{{name}}" or a missing
// "${ENV}" backing it is a config error — fail closed rather than sending
// a literal placeholder or an empty secret (ADR-015).
func (r *Resolver) Interpolate(s string) (string, error) {
	env := r.Current()
	var missing []string
	result := varPattern.ReplaceAllStringFunc(s, func(match string) string {
		name := varPattern.FindStringSubmatch(match)[1]
		if name == "base_url" {
			return env.BaseURL
		}
		raw, ok := env.Variables[name]
		if !ok {
			missing = append(missing, name)
			return match
		}
		expanded, err := r.expandOSEnv(raw)
		if err != nil {
			missing = append(missing, name+" ("+err.Error()+")")
			return match
		}
		return expanded
	})
	if len(missing) > 0 {
		return "", domain.NewConfigError(
			fmt.Sprintf("undefined variable(s) in %q: %s", s, strings.Join(missing, ", ")), nil)
	}
	return result, nil
}

// InterpolateRequest expands a RequestTemplate into a concrete HTTPRequest.
// It applies auth (if present) as headers/query before returning.
func (r *Resolver) InterpolateRequest(tmpl domain.RequestTemplate) (domain.HTTPRequest, error) {
	urlStr, err := r.Interpolate(tmpl.URL)
	if err != nil {
		return domain.HTTPRequest{}, err
	}

	headers := http.Header{}
	for k, v := range tmpl.Headers {
		val, err := r.Interpolate(v)
		if err != nil {
			return domain.HTTPRequest{}, err
		}
		headers.Set(k, val)
	}

	query := url.Values{}
	for k, v := range tmpl.Query {
		val, err := r.Interpolate(v)
		if err != nil {
			return domain.HTTPRequest{}, err
		}
		query.Set(k, val)
	}

	body, contentType, err := r.interpolateBody(tmpl.Body)
	if err != nil {
		return domain.HTTPRequest{}, err
	}
	if contentType != "" && headers.Get("Content-Type") == "" {
		headers.Set("Content-Type", contentType)
	}

	if tmpl.Auth != nil {
		if err := r.applyAuth(*tmpl.Auth, headers, query); err != nil {
			return domain.HTTPRequest{}, err
		}
	}

	return domain.HTTPRequest{
		Method:  tmpl.Method,
		URL:     urlStr,
		Headers: headers,
		Query:   query,
		Body:    body,
		Timeout: tmpl.Timeout,
	}, nil
}

func (r *Resolver) applyAuth(a domain.AuthTemplate, headers http.Header, query url.Values) error {
	scheme, ok := r.auth.Get(a.Type)
	if !ok {
		return auth.ErrUnknownKind(a.Type)
	}
	token, err := r.Interpolate(a.Token)
	if err != nil {
		return err
	}
	user, err := r.Interpolate(a.Username)
	if err != nil {
		return err
	}
	pass, err := r.Interpolate(a.Password)
	if err != nil {
		return err
	}
	val, err := r.Interpolate(a.Value)
	if err != nil {
		return err
	}
	creds := auth.Credentials{
		Token:    token,
		Username: user,
		Password: pass,
		Header:   a.Header,
		Value:    val,
		Name:     a.Name,
	}
	req := &domain.HTTPRequest{Headers: headers}
	if err := scheme.Apply(req, creds); err != nil {
		return err
	}
	return nil
}
