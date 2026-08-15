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
	"github.com/sandeepv/apilens/internal/graphqlop"
	"github.com/sandeepv/apilens/internal/security"
	"gopkg.in/yaml.v3"
)

// droppedRequestHeaders are never copied into the generated test
// (docs/06-test-dsl.md section 10). Accept-Encoding must be dropped:
// Go's HTTP client will not decompress the response if the test sets
// that header, so GraphQL assertions see gzip bytes and fail with
// "response is not JSON".
var droppedRequestHeaders = map[string]bool{
	"authorization":     true,
	"cookie":            true,
	"set-cookie":        true,
	"accept-encoding":   true,
	"content-encoding":  true,
	"content-length":    true,
	"transfer-encoding": true,
	"connection":        true,
	"proxy-connection":  true,
	"keep-alive":        true,
	"te":                true,
	"trailer":           true,
	"upgrade":           true,
	"host":              true,
	"origin":            true,
	"referer":           true,
	"user-agent":        true,
	"accept-language":   true,
}

func dropGeneratedHeader(name string) bool {
	n := strings.ToLower(name)
	if droppedRequestHeaders[n] {
		return true
	}
	return strings.HasPrefix(n, "sec-")
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

	content, err := MarshalYAML(tc)
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
	path := requestPath(ex.Request.URL)
	name := fmt.Sprintf("%s %s", ex.Request.Method, path)

	headers := map[string]string{}
	for k, v := range ex.Request.Headers {
		if dropGeneratedHeader(k) {
			continue
		}
		if len(v) > 0 {
			headers[k] = v[0]
		}
	}

	status := ex.Response.StatusCode
	tc := domain.TestCase{
		Name: name,
		Tags: []string{"generated"},
		Request: domain.RequestTemplate{
			Method:  ex.Request.Method,
			URL:     "{{base_url}}" + path,
			Headers: headers,
		},
		Assert: domain.AssertionSpec{
			Status: &domain.StatusSpec{Equals: &status},
		},
	}

	if payload, op, ok := graphqlop.ParseHTTPBody(ex.Request.Body); ok && !looksLikeLoginPayload(ex.Request.Body) {
		tc.Name = fmt.Sprintf("%s %s", graphqlop.DisplayMethod(op.Type), op.PrimaryField())
		tc.Tags = append(tc.Tags, "graphql")
		tc.Request.Method = "POST"
		tc.Request.URL = "{{base_url}}/graphql"
		tc.Request.GraphQL = &domain.GraphQLTemplate{
			Query:         payload.Query,
			Variables:     payload.Variables,
			OperationName: payload.OperationName,
		}
		// Authorization is never copied from the capture. Attach env auth
		// so `AUTH_TOKEN=... apilens run --tag generated` works on Stance.
		tc.Request.Auth = &domain.AuthTemplate{Type: "bearer", Token: "{{token}}"}
		noErrors := true
		hasData := true
		tc.Assert.GraphQL = &domain.GraphQLAssertSpec{NoErrors: &noErrors, HasData: &hasData}
		return tc
	}

	if len(ex.Request.Body) > 0 && !looksLikeLoginPayload(ex.Request.Body) {
		var parsed any
		if err := json.Unmarshal(ex.Request.Body, &parsed); err == nil {
			tc.Request.Body.JSON = parsed
		}
	}
	return tc
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
	if _, op, ok := graphqlop.ParseHTTPBody(ex.Request.Body); ok {
		filename := fmt.Sprintf("%s-%s.yaml", op.Type, slugPattern.ReplaceAllString(op.PrimaryField(), "-"))
		return filepath.Join(".apilens", "tests", "generated", filename)
	}
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
// to marshal a TestCase back to YAML. generate and recording share
// MarshalYAML so GraphQL, auth, and header-dropping cannot drift.
type yamlDocument struct {
	Version int         `yaml:"version"`
	ID      string      `yaml:"id,omitempty"`
	Name    string      `yaml:"name"`
	Tags    []string    `yaml:"tags,omitempty"`
	Request yamlRequest `yaml:"request"`
	Assert  yamlAssert  `yaml:"assert"`
}

type yamlRequest struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Auth    *yamlAuth         `yaml:"auth,omitempty"`
	GraphQL *yamlGraphQL      `yaml:"graphql,omitempty"`
	Body    *yamlBody         `yaml:"body,omitempty"`
}

type yamlAuth struct {
	Type  string `yaml:"type"`
	Token string `yaml:"token,omitempty"`
}

type yamlGraphQL struct {
	Query         string         `yaml:"query"`
	Variables     map[string]any `yaml:"variables,omitempty"`
	OperationName string         `yaml:"operation,omitempty"`
}

type yamlBody struct {
	JSON any `yaml:"json,omitempty"`
}

type yamlAssert struct {
	Status  yamlStatus     `yaml:"status"`
	GraphQL *yamlGQLAssert `yaml:"graphql,omitempty"`
}

type yamlStatus struct {
	Equals int `yaml:"equals"`
}

type yamlGQLAssert struct {
	NoErrors bool `yaml:"no_errors"`
	HasData  bool `yaml:"has_data"`
}

// MarshalYAML writes a TestCase as YAML. generate and recording share this
// so GraphQL, auth, and dropped headers cannot drift between `apilens
// generate` and `apilens record`.
func MarshalYAML(tc domain.TestCase) ([]byte, error) {
	ver := tc.Version
	if ver == 0 {
		ver = 1
	}
	doc := yamlDocument{
		Version: ver,
		ID:      tc.ID,
		Name:    tc.Name,
		Tags:    tc.Tags,
		Request: yamlRequest{
			Method:  string(tc.Request.Method),
			URL:     tc.Request.URL,
			Headers: tc.Request.Headers,
		},
	}
	if a := tc.Request.Auth; a != nil {
		doc.Request.Auth = &yamlAuth{Type: a.Type, Token: a.Token}
	}
	if g := tc.Request.GraphQL; g != nil {
		yg := &yamlGraphQL{Query: g.Query, OperationName: g.OperationName, Variables: asStringAnyMap(g.Variables)}
		doc.Request.GraphQL = yg
	} else if tc.Request.Body.JSON != nil {
		doc.Request.Body = &yamlBody{JSON: tc.Request.Body.JSON}
	}
	if tc.Assert.Status != nil && tc.Assert.Status.Equals != nil {
		doc.Assert.Status.Equals = *tc.Assert.Status.Equals
	}
	if g := tc.Assert.GraphQL; g != nil {
		doc.Assert.GraphQL = &yamlGQLAssert{}
		if g.NoErrors != nil {
			doc.Assert.GraphQL.NoErrors = *g.NoErrors
		}
		if g.HasData != nil {
			doc.Assert.GraphQL.HasData = *g.HasData
		}
	}
	return yaml.Marshal(doc)
}

func asStringAnyMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return nil
	}
	return m
}
