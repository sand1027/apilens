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

func containsStr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
