package specexport

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// Marshal serializes doc as YAML by round-tripping through JSON first —
// openapi3.T defines custom MarshalJSON (for its map-backed Paths/
// Responses types) but not MarshalYAML, so a direct yaml.Marshal would
// silently drop paths and responses. JSON->YAML round-trip preserves
// openapi3's marshaling logic while still producing readable YAML output.
func Marshal(doc *openapi3.T) ([]byte, error) {
	jsonBytes, err := json.Marshal(doc)
	if err != nil {
		return nil, domain.NewConfigError("marshaling OpenAPI document to JSON", err)
	}
	var generic any
	if err := json.Unmarshal(jsonBytes, &generic); err != nil {
		return nil, domain.NewConfigError("re-parsing OpenAPI JSON for YAML output", err)
	}
	out, err := yaml.Marshal(generic)
	if err != nil {
		return nil, domain.NewConfigError("marshaling OpenAPI document to YAML", err)
	}
	return out, nil
}

// WriteFile writes doc to path as YAML, creating parent directories as
// needed. Refuses to overwrite an existing file unless force is set,
// mirroring generate's overwrite policy (docs/08-proxy.md section 9).
func WriteFile(doc *openapi3.T, path string, force bool) error {
	if !force {
		if _, err := os.Stat(path); err == nil {
			return domain.NewConfigError(path+" already exists — pass --force to overwrite", nil)
		}
	}
	content, err := Marshal(doc)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewConfigError("creating output directory", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return domain.NewConfigError("writing OpenAPI export", err)
	}
	return nil
}
