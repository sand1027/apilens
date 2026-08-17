package generate

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

func sampleExchange() domain.Exchange {
	h := http.Header{}
	h.Set("Authorization", "Bearer real-secret")
	h.Set("Accept", "application/json")
	return domain.Exchange{
		Display: 42,
		Request: domain.HTTPRequest{
			Method:  "POST",
			URL:     "http://localhost:5000/api/forms",
			Headers: h,
			Body:    []byte(`{"title":"Example"}`),
		},
		Response: domain.HTTPResponse{StatusCode: 201},
	}
}

func TestFromExchange_WritesDefaultPath(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	g, err := svc.FromExchange(sampleExchange(), Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	want := filepath.Join(dir, ".apilens", "tests", "generated", "post-api-forms.yaml")
	if g.Path != want {
		t.Errorf("Path = %q, want %q", g.Path, want)
	}
	if _, err := os.Stat(g.Path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestFromExchange_DropsAuthorizationHeader(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	g, err := svc.FromExchange(sampleExchange(), Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if _, ok := g.Test.Request.Headers["Authorization"]; ok {
		t.Error("expected Authorization header to be dropped")
	}
	if _, ok := g.Test.Request.Headers["Accept"]; !ok {
		t.Error("expected non-sensitive Accept header to survive")
	}
	content := string(g.Content)
	if containsStr(content, "real-secret") {
		t.Errorf("generated YAML must never contain the real secret: %s", content)
	}
}

func TestFromExchange_DropsLoginLikeBody(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	ex := sampleExchange()
	ex.Request.Body = []byte(`{"username":"ada","password":"topsecret"}`)

	g, err := svc.FromExchange(ex, Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if g.Test.Request.Body.JSON != nil {
		t.Errorf("expected body to be dropped for a login-like payload, got %+v", g.Test.Request.Body.JSON)
	}
	content := string(g.Content)
	if containsStr(content, "topsecret") {
		t.Errorf("generated YAML must never contain a password: %s", content)
	}
}

func TestFromExchange_IncludesStatusAssertion(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	g, err := svc.FromExchange(sampleExchange(), Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if g.Test.Assert.Status == nil || g.Test.Assert.Status.Equals == nil || *g.Test.Assert.Status.Equals != 201 {
		t.Errorf("Assert.Status = %+v, want equals 201", g.Test.Assert.Status)
	}
}

func TestFromExchange_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.FromExchange(sampleExchange(), Options{}); err != nil {
		t.Fatalf("first FromExchange: %v", err)
	}
	_, err := svc.FromExchange(sampleExchange(), Options{})
	if err == nil {
		t.Fatal("expected error on second write without --force")
	}
}

func TestFromExchange_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	if _, err := svc.FromExchange(sampleExchange(), Options{}); err != nil {
		t.Fatalf("first FromExchange: %v", err)
	}
	_, err := svc.FromExchange(sampleExchange(), Options{Force: true})
	if err != nil {
		t.Fatalf("expected --force to allow overwrite: %v", err)
	}
}

func TestFromExchange_ExplicitOutPath(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	customPath := filepath.Join(dir, "custom", "my-test.yaml")
	g, err := svc.FromExchange(sampleExchange(), Options{Out: customPath})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if g.Path != customPath {
		t.Errorf("Path = %q, want %q", g.Path, customPath)
	}
}

func TestFromExchange_GeneratedYAMLParsesBack(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	g, err := svc.FromExchange(sampleExchange(), Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(g.Content, &doc); err != nil {
		t.Fatalf("generated YAML did not parse: %v\n%s", err, g.Content)
	}
	if doc["version"] != 1 {
		t.Errorf("version = %v, want 1", doc["version"])
	}
	req, ok := doc["request"].(map[string]any)
	if !ok {
		t.Fatalf("request block missing or wrong shape: %+v", doc)
	}
	if req["url"] != "{{base_url}}/api/forms" {
		t.Errorf("url = %v, want {{base_url}}/api/forms", req["url"])
	}
}

func TestFromExchange_GraphQLCapture(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	ex := sampleExchange()
	ex.Request.URL = "http://localhost:4000/graphql"
	ex.Request.Body = []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`)
	ex.Response.StatusCode = 200
	g, err := svc.FromExchange(ex, Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if g.Test.Request.GraphQL == nil || g.Test.Request.GraphQL.OperationName != "Ping" {
		t.Fatalf("GraphQL = %+v", g.Test.Request.GraphQL)
	}
	if g.Test.Assert.GraphQL == nil || g.Test.Assert.GraphQL.NoErrors == nil || !*g.Test.Assert.GraphQL.NoErrors {
		t.Fatalf("expected graphql.no_errors on generated test")
	}
	if !containsStr(string(g.Content), "no_errors") {
		t.Errorf("generated YAML missing graphql assertions:\n%s", g.Content)
	}
	want := filepath.Join(dir, ".apilens", "tests", "generated", "query-ping.yaml")
	if g.Path != want {
		t.Errorf("Path = %q, want %q", g.Path, want)
	}
}

func TestFromExchange_DropsBrowserNoiseAndAcceptEncoding(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	h := http.Header{}
	h.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	h.Set("Content-Length", "1093")
	h.Set("Proxy-Connection", "keep-alive")
	h.Set("User-Agent", "Mozilla/5.0")
	h.Set("Sec-Fetch-Mode", "cors")
	h.Set("Content-Type", "application/json")
	h.Set("X-Center-Id", "center-1")
	ex := sampleExchange()
	ex.Request.Headers = h
	g, err := svc.FromExchange(ex, Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if _, ok := g.Test.Request.Headers["Accept-Encoding"]; ok {
		t.Fatal("Accept-Encoding must be dropped (Go will not decompress if it is set)")
	}
	if _, ok := g.Test.Request.Headers["Content-Length"]; ok {
		t.Fatal("Content-Length must be dropped (body is rebuilt)")
	}
	if _, ok := g.Test.Request.Headers["User-Agent"]; ok {
		t.Fatal("User-Agent must be dropped")
	}
	if g.Test.Request.Headers["X-Center-Id"] != "center-1" {
		t.Fatalf("X-Center-Id should survive, got %v", g.Test.Request.Headers)
	}
	if g.Test.Request.Headers["Content-Type"] != "application/json" {
		t.Fatal("Content-Type should survive")
	}
}

func TestFromExchange_GraphQLAddsBearerAuthFromEnv(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	ex := sampleExchange()
	ex.Request.URL = "http://localhost:3000/graphql"
	ex.Request.Body = []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`)
	g, err := svc.FromExchange(ex, Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if g.Test.Request.Auth == nil || g.Test.Request.Auth.Type != "bearer" || g.Test.Request.Auth.Token != "{{token}}" {
		t.Fatalf("Auth = %+v, want bearer {{token}}", g.Test.Request.Auth)
	}
	if !containsStr(string(g.Content), "{{token}}") {
		t.Fatalf("generated YAML missing {{token}}:\n%s", g.Content)
	}
}

func TestFromExchange_DBHintsAddDBBlockToGraphQLCapture(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	ex := sampleExchange()
	ex.Request.URL = "http://localhost:3000/graphql"
	ex.Request.Body = []byte(`{"query":"mutation CreateAdvance($input: CreateAdvanceInput!) { createAdvance(input: $input) { _id total } }","operationName":"CreateAdvance"}`)
	ex.Response.StatusCode = 200
	ex.Response.Body = []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4","total":20000}}}`)

	g, err := svc.FromExchange(ex, Options{
		DBHints: &DBHintOptions{
			Connection: "main",
			Lister:     fakeCollectionLister{names: []string{"advances", "receipts"}},
		},
	})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if len(g.SkippedHints) != 0 {
		t.Errorf("expected no skipped hints, got %v", g.SkippedHints)
	}

	specs := g.Test.Assert.DB["main"]
	if len(specs) != 1 || specs[0].Collection != "advances" {
		t.Fatalf("expected a db check against advances, got %+v", specs)
	}

	// The written YAML must actually contain the db block, not just the
	// in-memory TestCase -- MarshalYAML previously dropped Assert.DB
	// entirely.
	content := string(g.Content)
	if !containsStr(content, "collection: advances") {
		t.Errorf("generated YAML missing the db.main.collection block:\n%s", content)
	}
	if !containsStr(content, "6a81dcfa6f6ebb3f76b802d4") {
		t.Errorf("generated YAML missing the _id filter value:\n%s", content)
	}
}

func TestFromExchange_DBHintsNilOptionSkipsEnrichmentEntirely(t *testing.T) {
	dir := t.TempDir()
	svc := New(dir)
	ex := sampleExchange()
	ex.Request.URL = "http://localhost:3000/graphql"
	ex.Request.Body = []byte(`{"query":"mutation CreateAdvance { createAdvance { _id } }"}`)
	ex.Response.Body = []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`)

	g, err := svc.FromExchange(ex, Options{}) // no DBHints at all
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if len(g.Test.Assert.DB) != 0 {
		t.Errorf("expected no db checks when DBHints is nil, got %+v", g.Test.Assert.DB)
	}
}

func TestMarshalYAML_RoundTripsDBBlock(t *testing.T) {
	exists := true
	tc := domain.TestCase{
		Name: "Roundtrip db block",
		Request: domain.RequestTemplate{
			Method: "GET",
			URL:    "{{base_url}}/health",
		},
		Assert: domain.AssertionSpec{
			Status: &domain.StatusSpec{Equals: intPtr(200)},
			DB: map[string][]domain.DBSpec{
				"main": {
					{Collection: "advances", Filter: map[string]any{"_id": "abc123"}, Exists: &exists},
				},
			},
		},
	}
	out, err := MarshalYAML(tc)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}

	var doc struct {
		Assert struct {
			DB map[string][]struct {
				Collection string `yaml:"collection"`
				Filter     any    `yaml:"filter"`
				Exists     bool   `yaml:"exists"`
			} `yaml:"db"`
		} `yaml:"assert"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("re-parsing generated YAML: %v\n%s", err, out)
	}
	specs, ok := doc.Assert.DB["main"]
	if !ok || len(specs) != 1 {
		t.Fatalf("expected db.main with 1 entry after round-trip, got %+v\n%s", doc.Assert.DB, out)
	}
	if specs[0].Collection != "advances" || !specs[0].Exists {
		t.Errorf("round-tripped spec = %+v", specs[0])
	}
}

func intPtr(i int) *int { return &i }

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
