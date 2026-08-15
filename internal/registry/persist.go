package registry

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// fileDocument mirrors the on-disk shape from docs/07-discovery.md
// section 8.
type fileDocument struct {
	Version     int            `yaml:"version"`
	GeneratedAt time.Time      `yaml:"generated_at"`
	Endpoints   []fileEndpoint `yaml:"endpoints"`
}

type fileEndpoint struct {
	Method  string   `yaml:"method"`
	Path    string   `yaml:"path"`
	Sources []string `yaml:"sources"`
	Tags    []string `yaml:"tags,omitempty"`
}

// SaveYAML writes endpoints to path in the documented registry.yaml shape
// (docs/07-discovery.md section 8). It always replaces the file — the
// cache is not hand-merged (ADR-017).
func SaveYAML(path string, endpoints []domain.Endpoint) error {
	sorted := make([]domain.Endpoint, len(endpoints))
	copy(sorted, endpoints)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Path != sorted[j].Path {
			return sorted[i].Path < sorted[j].Path
		}
		return sorted[i].Method < sorted[j].Method
	})

	doc := fileDocument{Version: 1, GeneratedAt: time.Now().UTC()}
	for _, ep := range sorted {
		doc.Endpoints = append(doc.Endpoints, fileEndpoint{
			Method:  string(ep.Method),
			Path:    ep.Path,
			Sources: ep.Sources,
			Tags:    ep.Tags,
		})
	}

	out, err := yaml.Marshal(doc)
	if err != nil {
		return domain.NewConfigError("marshaling registry.yaml", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return domain.NewConfigError("creating registry directory", err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return domain.NewConfigError("writing registry.yaml", err)
	}
	return nil
}

// LoadYAML reads a previously written registry.yaml. A missing file
// returns (nil, nil) — an empty registry is valid before the first
// discover (docs/07-discovery.md section 8).
func LoadYAML(path string) ([]domain.Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewConfigError("reading registry.yaml", err)
	}
	var doc fileDocument
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, domain.NewConfigError("parsing registry.yaml", err)
	}
	out := make([]domain.Endpoint, 0, len(doc.Endpoints))
	for _, fe := range doc.Endpoints {
		method := domain.NormalizeMethod(fe.Method)
		out = append(out, domain.Endpoint{
			ID:            domain.NewEndpointID(method, fe.Path),
			Method:        method,
			Path:          fe.Path,
			Sources:       fe.Sources,
			PrimarySource: primaryOf(fe.Sources),
			Tags:          fe.Tags,
		})
	}
	return out, nil
}

func primaryOf(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	return sources[0]
}
