package openapigen

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// WriteOptions configures WriteAll.
type WriteOptions struct {
	Dir   string // output directory; empty defaults to .apilens/tests/generated
	Force bool   // overwrite existing files
}

// WriteAll writes each Generated test to its own file under opts.Dir,
// named "<method>-<slug-path>.yaml" (same convention
// internal/generate.defaultRelativePath already uses for capture-derived
// tests, so files from both sources sit comfortably side by side and
// don't collide unless they really are the same method+path).
func WriteAll(generated []Generated, opts WriteOptions) ([]string, error) {
	if opts.Dir == "" {
		return nil, domain.NewConfigError("openapigen.WriteAll requires an output directory", nil)
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, domain.NewConfigError("creating openapigen output directory", err)
	}

	var written []string
	for _, g := range generated {
		filename := defaultFilename(g.Test)
		path := filepath.Join(opts.Dir, filename)
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				return written, domain.NewConfigError(
					fmt.Sprintf("%s already exists — pass --force to overwrite", path), nil)
			}
		}
		content, err := marshalTest(g.Test)
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return written, domain.NewConfigError("writing generated test "+path, err)
		}
		written = append(written, path)
	}
	return written, nil
}

var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// defaultFilename mirrors internal/generate's own naming convention
// ("<method>-<slug-path>.yaml") using the interpolated URL template
// (still containing "{{base_url}}", which the slug pattern just strips
// out along with every other non-alphanumeric run).
func defaultFilename(tc domain.TestCase) string {
	method := strings.ToLower(string(tc.Request.Method))
	slug := strings.Trim(slugPattern.ReplaceAllString(tc.Request.URL, "-"), "-")
	slug = strings.TrimPrefix(slug, "base-url-")
	if slug == "" {
		slug = "root"
	}
	return fmt.Sprintf("%s-%s.yaml", method, slug)
}

// yamlDoc mirrors internal/testdef's document shape closely enough to
// round-trip a v1 TestCase. Kept local for the same reason
// internal/generate and internal/recording each keep their own: yaml.
// Marshal needs a decode-shaped struct with omitempty tags, not
// testdef's parse-oriented (`any`-heavy) types.
type yamlDoc struct {
	Version     int         `yaml:"version"`
	Name        string      `yaml:"name"`
	Description string      `yaml:"description,omitempty"`
	Tags        []string    `yaml:"tags,omitempty"`
	Request     yamlRequest `yaml:"request"`
	Assert      yamlAssert  `yaml:"assert"`
}

type yamlRequest struct {
	Method string            `yaml:"method"`
	URL    string            `yaml:"url"`
	Query  map[string]string `yaml:"query,omitempty"`
	Body   *yamlBody         `yaml:"body,omitempty"`
}

type yamlBody struct {
	JSON any `yaml:"json,omitempty"`
}

type yamlAssert struct {
	Status yamlStatus `yaml:"status"`
}

type yamlStatus struct {
	Equals int `yaml:"equals"`
}

func marshalTest(tc domain.TestCase) ([]byte, error) {
	doc := yamlDoc{
		Version:     1,
		Name:        tc.Name,
		Description: tc.Description,
		Tags:        tc.Tags,
		Request: yamlRequest{
			Method: string(tc.Request.Method),
			URL:    tc.Request.URL,
			Query:  tc.Request.Query,
		},
	}
	if tc.Request.Body.JSON != nil {
		doc.Request.Body = &yamlBody{JSON: tc.Request.Body.JSON}
	}
	if tc.Assert.Status != nil && tc.Assert.Status.Equals != nil {
		doc.Assert.Status.Equals = *tc.Assert.Status.Equals
	}
	return yaml.Marshal(doc)
}
