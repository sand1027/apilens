package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
	"github.com/sandeepv/apilens/internal/testdef"
)

func TestGenerateCompileRun_GraphQLGzipHeaderAndBearerToken(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"errors":[{"message":"unauthorized"}]}`))
			return
		}
		payload := []byte(`{"data":{"ping":{"message":"ok"}}}`)
		if strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Content-Type", "application/json")
			gz := gzip.NewWriter(w)
			_, _ = gz.Write(payload)
			_ = gz.Close()
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(payload)
	}))
	defer ts.Close()

	dir := t.TempDir()
	envDir := filepath.Join(dir, ".apilens", "environments")
	if err := os.MkdirAll(envDir, 0o755); err != nil {
		t.Fatal(err)
	}
	envYAML := "base_url: " + ts.URL + "\nvariables:\n  token: test-token\n"
	if err := os.WriteFile(filepath.Join(envDir, "local.yaml"), []byte(envYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	h := http.Header{}
	h.Set("Accept-Encoding", "gzip, deflate, br")
	h.Set("Authorization", "Bearer captured-secret")
	h.Set("Content-Type", "application/json")
	ex := domain.Exchange{
		Request: domain.HTTPRequest{
			Method:  "POST",
			URL:     ts.URL + "/graphql",
			Headers: h,
			Body:    []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`),
		},
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"ping":{"message":"ok"}}}`)},
	}

	g, err := generate.New(dir).FromExchange(ex, generate.Options{})
	if err != nil {
		t.Fatalf("FromExchange: %v", err)
	}
	if _, ok := g.Test.Request.Headers["Accept-Encoding"]; ok {
		t.Fatal("Accept-Encoding must be stripped so the runner can decompress JSON")
	}
	if g.Test.Request.Auth == nil || g.Test.Request.Auth.Token != "{{token}}" {
		t.Fatalf("Auth = %+v, want bearer {{token}}", g.Test.Request.Auth)
	}
	if bytes.Contains(g.Content, []byte("captured-secret")) {
		t.Fatalf("generated YAML leaked the captured token:\n%s", g.Content)
	}

	tc, err := testdef.Compile(g.Content, g.Path)
	if err != nil {
		t.Fatalf("Compile: %v\n%s", err, g.Content)
	}
	if tc.Request.GraphQL == nil || tc.Request.Auth == nil {
		t.Fatalf("compiled test missing graphql/auth: %+v", tc.Request)
	}

	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Env.Use("local"); err != nil {
		t.Fatal(err)
	}

	report, err := a.RunSuite(context.Background(), RunFilter{}, nil)
	if err != nil {
		t.Fatalf("RunSuite: %v", err)
	}
	if report.Counts.Failed > 0 || report.Counts.Errored > 0 || report.Counts.Passed == 0 {
		var reasons []string
		for _, r := range report.Results {
			reasons = append(reasons, r.Name+": "+string(r.Status)+" "+joinAsserts(r))
		}
		t.Fatalf("report counts=%+v results=%v", report.Counts, reasons)
	}
}

func joinAsserts(r domain.TestResult) string {
	var b strings.Builder
	for _, a := range r.Assertions {
		if !a.Passed {
			b.WriteString(a.Reason)
			b.WriteString("; ")
		}
	}
	if r.Error != "" {
		b.WriteString(r.Error)
	}
	return b.String()
}
