package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// ValidateDBDriver rejects a driver name internal/dbassert cannot open,
// so `apilens configure` fails immediately with a clear message instead
// of writing a config.yaml that only breaks the next `apilens run`.
// Mirrors dbassert.registeredDriverName's allow-list; duplicated rather
// than imported to keep internal/config free of a dependency on
// internal/dbassert (docs/02-packages.md section 2's one-way dependency
// direction -- dbassert already depends on nothing from config besides
// the plain DBConnectionConfig struct fields it's handed).
func ValidateDBDriver(name string) error {
	switch strings.ToLower(name) {
	case "sqlite", "sqlite3", "postgres", "postgresql", "pgx", "mongodb", "mongo":
		return nil
	default:
		return fmt.Errorf("unsupported db driver %q (supported: sqlite, postgres, mongodb)", name)
	}
}

// SetDBConnection writes/updates a single db.connections.<name> entry in
// the config.yaml at path, preserving every other section of the file
// (comments included, best-effort -- yaml.v3's Node API round-trips
// existing structure rather than re-marshaling a plain Go struct, which
// would drop every comment in the file).
//
// The DSN is deliberately NOT written as a literal value here -- it is
// always "${dsnEnvVar}" so the actual secret lives only in
// .apilens/.secrets.env (internal/secrets), never in a file a project
// might accidentally commit (docs/09-security.md's "never in git"
// posture, same as every other DSN in this codebase).
func SetDBConnection(path, name, driver, dsnEnvVar string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("reading %s: %w", path, err)
		}
		raw = nil // no config.yaml yet (e.g. configure ran before init) -- start from an empty document
	}

	var doc yaml.Node
	if len(raw) > 0 {
		if err := yaml.Unmarshal(raw, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	}
	if len(doc.Content) == 0 {
		doc.Kind = yaml.DocumentNode
		doc.Content = []*yaml.Node{newMappingNode()}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return fmt.Errorf("%s: expected a YAML mapping at the document root", path)
	}

	dbNode := findOrCreateChildMap(root, "db")
	connectionsNode := findOrCreateChildMap(dbNode, "connections")
	connNode := findOrCreateChildMap(connectionsNode, name)
	setScalarKey(connNode, "driver", driver)
	setScalarKey(connNode, "dsn", "${"+dsnEnvVar+"}")

	out, err := yaml.Marshal(&doc)
	if err != nil {
		return fmt.Errorf("encoding %s: %w", path, err)
	}
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("creating %s: %w", dir, err)
		}
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

func newMappingNode() *yaml.Node {
	return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
}

// findOrCreateChildMap returns the mapping node under parent[key],
// creating an empty mapping and appending the key/value pair if it did
// not already exist. parent must be a MappingNode.
func findOrCreateChildMap(parent *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			if parent.Content[i+1].Kind != yaml.MappingNode {
				// Was a scalar/null (e.g. commented-out placeholder
				// decoded as null) -- replace with a mapping so nested
				// keys have somewhere to live.
				parent.Content[i+1] = newMappingNode()
			}
			return parent.Content[i+1]
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := newMappingNode()
	parent.Content = append(parent.Content, keyNode, valNode)
	return valNode
}

// setScalarKey sets parent[key] = value as a plain scalar, updating it in
// place if the key already exists or appending it otherwise.
func setScalarKey(parent *yaml.Node, key, value string) {
	for i := 0; i+1 < len(parent.Content); i += 2 {
		if parent.Content[i].Value == key {
			parent.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
			return
		}
	}
	keyNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key}
	valNode := &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
	parent.Content = append(parent.Content, keyNode, valNode)
}
