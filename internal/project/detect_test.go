package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectProject_GraphQLFromSchemaFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "schema.graphql"), []byte("type Query { x: String }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d := detectProject(dir)
	if !d.GraphQL {
		t.Fatal("expected GraphQL detect")
	}
	if d.BaseURL != "http://localhost:3000" {
		t.Errorf("BaseURL = %q, want graphql local default", d.BaseURL)
	}
}

func TestDetectProject_BaseURLFromEnvGraphQLVar(t *testing.T) {
	dir := t.TempDir()
	env := "NEXT_PUBLIC_GRAPHQL_API_URL=http://localhost:3000/graphql\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(env), 0o644); err != nil {
		t.Fatal(err)
	}
	d := detectProject(dir)
	if d.BaseURL != "http://localhost:3000" {
		t.Errorf("BaseURL = %q", d.BaseURL)
	}
}

func TestOriginOfStripsPath(t *testing.T) {
	if got := originOf("http://localhost:3000/graphql"); got != "http://localhost:3000" {
		t.Errorf("got %q", got)
	}
}
