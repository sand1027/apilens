// Package secrets manages ".apilens/.secrets.env", a gitignored,
// dotenv-style file for values that must never be committed (auth
// tokens, database DSNs) but still need to persist across CLI
// invocations -- unlike an OS-exported env var (only lives for one
// shell session) or an in-memory store (only lives for one process).
//
// This file is loaded once per process, early in internal/app.New,
// before config.Load or environment.LoadDir run, so "${AUTH_TOKEN}" /
// "${DATABASE_URL}" placeholders already used throughout config.yaml
// and .apilens/environments/*.yaml resolve without the user
// re-exporting them in every shell. An explicit process environment
// variable of the same name always wins (Load never overwrites an
// already-set var), preserving the existing "OS env overlays file"
// precedence used elsewhere (internal/environment.loadFile).
package secrets

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Path returns the location of the secrets file for a project rooted at
// projectDir.
func Path(projectDir string) string {
	return filepath.Join(projectDir, ".apilens", ".secrets.env")
}

// Load reads path (KEY=VALUE per line; blank lines and "#"-prefixed
// lines are ignored) and applies each key to the process environment via
// os.Setenv -- but only when that key is not already set, so an explicit
// "export FOO=bar" before invoking apilens always takes priority over
// the persisted file. A missing file is not an error: it only exists
// once `apilens configure` has written it.
func Load(path string) error {
	kv, err := readAll(path)
	if err != nil {
		return err
	}
	for k, v := range kv {
		if _, isSet := os.LookupEnv(k); !isSet {
			_ = os.Setenv(k, v)
		}
	}
	return nil
}

// ReadEnvFile parses any dotenv-style file (KEY=VALUE per line, "#"
// comments, optional surrounding quotes on the value) into a map,
// without touching the process environment. Exported so callers like
// `apilens configure --env-file` can pull one named value (e.g. an
// app's existing MONGO_CONNECTION_URL) straight out of a project's own
// .env file, rather than requiring the user to "export" it into the
// shell first just to hand it to a --flag (which would put the secret
// in shell history either way).
func ReadEnvFile(path string) (map[string]string, error) {
	return readAll(path)
}

// Set merges kv into the file at path (creating it and its parent
// directory if needed) and rewrites it. Existing keys not present in kv
// are preserved, so running `apilens configure` for the database alone
// does not erase a previously configured AUTH_TOKEN.
func Set(path string, kv map[string]string) error {
	existing, err := readAll(path)
	if err != nil {
		return err
	}
	for k, v := range kv {
		existing[k] = v
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating %s: %w", filepath.Dir(path), err)
	}

	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString("# Managed by `apilens configure`. Never commit this file --\n")
	b.WriteString("# it is gitignored by `apilens init`.\n")
	for _, k := range keys {
		fmt.Fprintf(&b, "%s=%s\n", k, existing[k])
	}

	// 0o600: this file holds credentials, unlike config.yaml/environment
	// files which are safe to be group/world-readable.
	if err := os.WriteFile(path, []byte(b.String()), 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func readAll(path string) (map[string]string, error) {
	out := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value = strings.Trim(strings.TrimSpace(value), `"`)
		out[key] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	return out, nil
}
