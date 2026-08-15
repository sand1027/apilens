// Package generate implements docs/08-proxy.md section 9: map a captured
// Exchange to a YAML v1 test file. Secrets are dropped, never copied from
// the capture — the author attaches auth via the environment
// (docs/06-test-dsl.md section 10, docs/11-risks-and-gaps.md G11).
package generate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/security"
	"gopkg.in/yaml.v3"
)

// droppedRequestHeaders are never copied into the generated test
// (docs/06-test-dsl.md section 10).
var droppedRequestHeaders = map[string]bool{
	"authorization": true,
	"cookie":        true,
	"set-cookie":    true,
}

// Options configures Generate (docs/04-interfaces.md section 12).
type Options struct {
	Out   string // explicit output path; empty uses the default slug path
	Force bool   // overwrite an existing file
}

// Generated mirrors docs/04-interfaces.md section 12's generate.Generated.
type Generated struct {
	Path    string
	Content []byte
	Test    domain.TestCase
}

// Service implements docs/04-interfaces.md section 12's generate.Service.
type Service struct {
	projectDir string
}

// New builds a generate Service rooted at projectDir (used to resolve the
// default `.apilens/tests/generated/...` path).
func New(projectDir string) *Service {
	return &Service{projectDir: projectDir}
}

// FromExchange builds a domain.TestCase from ex and writes it to disk,
// refusing to overwrite an existing file unless opts.Force
// (docs/08-proxy.md section 9).
func (s *Service) FromExchange(ex domain.Exchange, opts Options) (Generated, error) {
	tc := BuildTestCase(ex)

	path := opts.Out
	if path == "" {
		path = filepath.Join(s.projectDir, defaultRelativePath(ex))
	}

	if !opts.Force {
		if _, err := os.Stat(path); err == nil {
			return Generated{}, domain.NewConfigError(
				fmt.Sprintf("%s already exists — pass --force to overwrite", path), nil)
		}
	}

	content, err := marshalTest(tc)
	if err != nil {
		return Generated{}, err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Generated{}, domain.NewConfigError("creating tests directory", err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		return Generated{}, domain.NewConfigError("writing generated test", err)
	}

	return Generated{Path: path, Content: content, Test: tc}, nil
}

// BuildTestCase maps an Exchange to a TestCase per docs/06-test-dsl.md
// section 10 and docs/08-proxy.md section 9: method, URL, safe headers,
// body (if non-sensitive), and a status.equals assertion on the captured
// status. Exported so internal/recording (plan.md v9's recording
// sessions) can reuse the exact same header-dropping / login-payload
// rules rather than duplicating them with a chance to drift.
func BuildTestCase(ex domain.Exchange) domain.TestCase {
	name := fmt.Sprintf("%s %s", ex.Request.Method, requestPath(ex.Request.URL))

	headers := map[string]string{}
	for k, v := range ex.Request.Headers {
		if droppedRequestHeaders[strings.ToLower(k)] {
			continue
		}
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	body := domain.BodyTemplate{}
	if len(ex.Request.Body) > 0 && !looksLikeLoginPayload(ex.Request.Body) {
		var parsed any
		if err := json.Unmarshal(ex.Request.Body, &parsed); err == nil {
			body.JSON = parsed
		}
	}

	status := ex.Response.StatusCode
	return domain.TestCase{
		Name: name,
		Tags: []string{"generated"},
		Request: domain.RequestTemplate{
			Method:  ex.Request.Method,
			URL:     "{{base_url}}" + requestPath(ex.Request.URL),
			Headers: headers,
			Body:    body,
		},
		Assert: domain.AssertionSpec{
			Status: &domain.StatusSpec{Equals: &status},
		},
	}
}

// looksLikeLoginPayload implements docs/06-test-dsl.md section 10: "Body
// dropped if it looks like a login payload (keys password, token, secret,
// client_secret)". Reuses the same sensitive-key detection the redactor
// already applies to bodies, so generate and display agree on what counts
// as sensitive.
func looksLikeLoginPayload(body []byte) bool {
	return security.ContainsSensitiveBodyKey(body)
}

func requestPath(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			rawURL = rest[slash:]
		} else {
			rawURL = "/"
		}
	}
	if q := strings.IndexByte(rawURL, '?'); q >= 0 {
		rawURL = rawURL[:q]
	}
	if rawURL == "" {
		rawURL = "/"
	}
	return rawURL
}

// slugPattern replaces anything that isn't alphanumeric with a hyphen for
// the default generated filename.
var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// defaultRelativePath implements docs/08-proxy.md section 9's default:
// ".apilens/tests/generated/<method>-<slug-path>.yaml", e.g.
// "POST /api/forms" -> "generated/post-api-forms.yaml".
func defaultRelativePath(ex domain.Exchange) string {
	method := strings.ToLower(string(ex.Request.Method))
	path := requestPath(ex.Request.URL)
	slug := strings.Trim(slugPattern.ReplaceAllString(path, "-"), "-")
	if slug == "" {
		slug = "root"
	}
	filename := fmt.Sprintf("%s-%s.yaml", method, slug)
	return filepath.Join(".apilens", "tests", "generated", filename)
}

// yamlDocument mirrors internal/testdef's document shape closely enough
// to marshal a TestCase back to the documented YAML v1 form. Kept local
// (rather than importing internal/testdef) since generate only needs to
// write, not parse, and testdef's raw types are decode-oriented (`any`
// heavy) in ways that don't round-trip cleanly through yaml.Marshal.
type yamlDocument struct {
	Version int         `yaml:"version"`
	Name    string      `yaml:"name"`
	Tags    []string    `yaml:"tags,omitempty"`
	Request yamlRequest `yaml:"request"`
	Assert  yamlAssert  `yaml:"assert"`
}

type yamlRequest struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body    *yamlBody         `yaml:"body,omitempty"`
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
	doc := yamlDocument{
		Version: 1,
		Name:    tc.Name,
		Tags:    tc.Tags,
		Request: yamlRequest{
			Method:  string(tc.Request.Method),
			URL:     tc.Request.URL,
			Headers: tc.Request.Headers,
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
