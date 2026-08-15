package recording

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// WriteOptions configures WriteSuite.
type WriteOptions struct {
	Dir   string // output directory; files are named "01-<id>.yaml", "02-<id>.yaml", ...
	Force bool   // overwrite existing files
}

// WriteSuite writes each Step to its own numbered YAML v2 file in
// opts.Dir, preserving the recorded order in the filenames (the loader
// sorts by file path, so "01-..." must run before "02-..." — same
// determinism guarantee testdef.Loader.LoadAll already provides, just
// made explicit for a generated suite instead of relying on the author's
// own naming).
func WriteSuite(steps []Step, opts WriteOptions) ([]string, error) {
	if opts.Dir == "" {
		return nil, domain.NewConfigError("recording.WriteSuite requires an output directory", nil)
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, domain.NewConfigError("creating recording output directory", err)
	}

	var written []string
	for i, step := range steps {
		filename := fmt.Sprintf("%02d-%s.yaml", i+1, step.ID)
		path := filepath.Join(opts.Dir, filename)
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				return written, domain.NewConfigError(
					fmt.Sprintf("%s already exists — pass --force to overwrite", path), nil)
			}
		}
		content, err := marshalStep(step)
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return written, domain.NewConfigError("writing recorded test "+path, err)
		}
		written = append(written, path)
	}
	return written, nil
}

// yamlDoc mirrors internal/testdef's document shape closely enough to
// round-trip a DSL v2 TestCase (version + id fields included, unlike
// internal/generate's own yamlDocument which is v1-only). Kept local for
// the same reason generate keeps its own: yaml.Marshal needs a
// decode-shaped struct with omitempty tags, not testdef's parse-oriented
// types.
type yamlDoc struct {
	Version int            `yaml:"version"`
	ID      string         `yaml:"id,omitempty"`
	Name    string         `yaml:"name"`
	Tags    []string       `yaml:"tags,omitempty"`
	Request yamlRequestDoc `yaml:"request"`
	Assert  yamlAssertDoc  `yaml:"assert"`
}

type yamlRequestDoc struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    *yamlBodyDoc      `yaml:"body,omitempty"`
}

type yamlBodyDoc struct {
	JSON any `yaml:"json,omitempty"`
}

type yamlAssertDoc struct {
	Status yamlStatusDoc `yaml:"status"`
}

type yamlStatusDoc struct {
	Equals int `yaml:"equals"`
}

func marshalStep(step Step) ([]byte, error) {
	tc := step.Test
	doc := yamlDoc{
		Version: tc.Version,
		ID:      tc.ID,
		Name:    tc.Name,
		Tags:    tc.Tags,
		Request: yamlRequestDoc{
			Method:  string(tc.Request.Method),
			URL:     tc.Request.URL,
			Headers: tc.Request.Headers,
		},
	}
	if tc.Request.Body.JSON != nil {
		doc.Request.Body = &yamlBodyDoc{JSON: tc.Request.Body.JSON}
	}
	if tc.Assert.Status != nil && tc.Assert.Status.Equals != nil {
		doc.Assert.Status.Equals = *tc.Assert.Status.Equals
	}
	return yaml.Marshal(doc)
}
