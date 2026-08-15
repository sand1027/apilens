package graphql

import (
	"context"
	"os"
	"testing"
	"testing/fstest"

	"github.com/sandeepv/apilens/internal/discovery"
)

func TestDetectAndDiscoverFixtureSchema(t *testing.T) {
	raw, err := os.ReadFile("../../../../testdata/graphql/schema.graphql")
	if err != nil {
		t.Fatal(err)
	}
	root := fstest.MapFS{"schema.graphql": {Data: raw}}
	p := New(nil)
	ok, err := p.Detect(context.Background(), root)
	if err != nil || !ok {
		t.Fatalf("Detect: ok=%v err=%v", ok, err)
	}
	eps, err := p.Discover(context.Background(), root, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"/graphql/query/ping":            "QUERY",
		"/graphql/query/appVersionInfo":  "QUERY",
		"/graphql/query/centers":         "QUERY",
		"/graphql/mutation/pong":         "MUTATION",
	}
	if len(eps) != len(want) {
		t.Fatalf("got %d endpoints: %+v", len(eps), eps)
	}
	for _, ep := range eps {
		method, ok := want[ep.Path]
		if !ok {
			t.Errorf("unexpected %s %s", ep.Method, ep.Path)
			continue
		}
		if string(ep.Method) != method {
			t.Errorf("%s method = %s, want %s", ep.Path, ep.Method, method)
		}
		if ep.PrimarySource != "graphql" {
			t.Errorf("source = %s", ep.PrimarySource)
		}
	}
}

func TestDiscoverSkipsWhenNoSchema(t *testing.T) {
	p := New(nil)
	ok, err := p.Detect(context.Background(), fstest.MapFS{"readme.md": {Data: []byte("hi")}})
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no detect")
	}
}

func TestDiscoverMergedExtendFiles(t *testing.T) {
	root := fstest.MapFS{
		"a.graphql": {Data: []byte("type Query { ping: String! }\n")},
		"b.graphql": {Data: []byte("extend type Query { centers: String! }\ntype Mutation { pong: String! }\n")},
	}
	p := New(nil)
	eps, err := p.Discover(context.Background(), root, discovery.Options{})
	if err != nil {
		t.Fatal(err)
	}
	paths := map[string]bool{}
	for _, ep := range eps {
		paths[ep.Path] = true
	}
	if !paths["/graphql/query/ping"] || !paths["/graphql/query/centers"] || !paths["/graphql/mutation/pong"] {
		t.Fatalf("missing fields: %v", paths)
	}
}

func TestDiscoverFallsBackWhenMergedSchemaIsTruncated(t *testing.T) {
	root := fstest.MapFS{
		"schema.graphql": {Data: []byte("type Query {\n  ping: String!\n}\n\n\t\"\"\"\n")},
		"apps/api/src/modules/center/schema.graphql": {Data: []byte("extend type Query { centers: [String!]! }\n")},
		"apps/api/src/modules/auth/schema.graphql":   {Data: []byte("extend type Mutation { login: String! }\n")},
	}
	p := New(nil)
	eps, err := p.Discover(context.Background(), root, discovery.Options{})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	paths := map[string]bool{}
	for _, ep := range eps {
		paths[ep.Path] = true
	}
	if !paths["/graphql/query/centers"] || !paths["/graphql/mutation/login"] {
		t.Fatalf("expected fallback to module SDL, got %v", paths)
	}
	if paths["/graphql/query/ping"] {
		t.Fatal("truncated merged schema must not contribute fields")
	}
}

func TestDiscoverExplicitPathReportsParseError(t *testing.T) {
	root := fstest.MapFS{
		"schema.graphql": {Data: []byte("type Query {\n  ping: String!\n}\n\n\t\"\"\"\n")},
	}
	p := New(nil)
	_, err := p.Discover(context.Background(), root, discovery.Options{Paths: []string{"schema.graphql"}})
	if err == nil {
		t.Fatal("expected parse error for --path pointing at truncated SDL")
	}
}
