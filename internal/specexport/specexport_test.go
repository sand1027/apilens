package specexport

import (
	"net/http"
	"strings"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestBuild_GroupsMultipleMethodsUnderOnePath(t *testing.T) {
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/api/users"},
		{Method: "POST", Path: "/api/users"},
	}
	doc := Build(endpoints, Options{})
	item := doc.Paths.Find("/api/users")
	if item == nil {
		t.Fatal("expected /api/users to be present")
	}
	if item.Get == nil || item.Post == nil {
		t.Fatalf("expected both GET and POST on /api/users, got %+v", item)
	}
}

func TestBuild_NormalizesPathParamStyles(t *testing.T) {
	// :id (Express/Gin) and {id} (OpenAPI) both normalize to the same
	// canonical path (domain.NormalizePath), so they must collapse into
	// one PathItem rather than two.
	endpoints := []domain.Endpoint{
		{Method: "GET", Path: "/api/users/:id"},
	}
	doc := Build(endpoints, Options{})
	if doc.Paths.Len() != 1 {
		t.Fatalf("expected exactly 1 path, got %d", doc.Paths.Len())
	}
}

func TestBuild_PreservesOperationIDAndTagsFromSpec(t *testing.T) {
	endpoints := []domain.Endpoint{
		{
			Method: "GET", Path: "/api/users",
			Tags: []string{"users"},
			Spec: &domain.EndpointSpec{Name: "listUsers"},
		},
	}
	doc := Build(endpoints, Options{})
	op := doc.Paths.Find("/api/users").Get
	if op.OperationID != "listUsers" {
		t.Errorf("OperationID = %q, want listUsers", op.OperationID)
	}
	if len(op.Tags) != 1 || op.Tags[0] != "users" {
		t.Errorf("Tags = %v", op.Tags)
	}
}

func TestBuild_UsesCapturedExampleForStatusAndSchema(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users/:id"}}
	ex := domain.Exchange{
		Response: domain.HTTPResponse{
			StatusCode: 201,
			Headers:    http.Header{},
			Body:       []byte(`{"id":1,"name":"Ada"}`),
		},
	}
	examples := map[domain.EndpointID]domain.Exchange{
		domain.NewEndpointID("GET", "/api/users/:id"): ex,
	}
	doc := Build(endpoints, Options{Examples: examples})
	op := doc.Paths.Find("/api/users/:id").Get
	if op.Responses.Status(201) == nil {
		t.Fatal("expected a 201 response entry from the captured example")
	}
	resp := op.Responses.Status(201).Value
	mt := resp.Content.Get("application/json")
	if mt == nil || mt.Schema == nil || mt.Schema.Value == nil {
		t.Fatal("expected an inferred JSON schema on the 201 response")
	}
	if mt.Schema.Value.Type == nil || !mt.Schema.Value.Type.Is("object") {
		t.Errorf("expected inferred schema type object, got %v", mt.Schema.Value.Type)
	}
}

func TestBuild_NoExampleDefaultsTo200WithNoSchema(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/ping"}}
	doc := Build(endpoints, Options{})
	op := doc.Paths.Find("/api/ping").Get
	ref := op.Responses.Status(200)
	if ref == nil {
		t.Fatal("expected a default 200 response")
	}
	if len(ref.Value.Content) != 0 {
		t.Errorf("expected no content/schema without a captured example, got %+v", ref.Value.Content)
	}
}

func TestBuild_SetsBaseURLAsServerHint(t *testing.T) {
	doc := Build(nil, Options{BaseURL: "http://localhost:5050"})
	if len(doc.Servers) != 1 || doc.Servers[0].URL != "http://localhost:5050" {
		t.Errorf("Servers = %+v", doc.Servers)
	}
}

func TestMarshal_ProducesParseableYAMLWithPaths(t *testing.T) {
	endpoints := []domain.Endpoint{{Method: "GET", Path: "/api/users"}}
	doc := Build(endpoints, Options{})
	out, err := Marshal(doc)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	// The JSON->YAML round-trip is exactly the failure mode we guard
	// against: a direct yaml.Marshal(doc) would silently drop paths.
	s := string(out)
	if !strings.Contains(s, "/api/users") {
		t.Errorf("expected marshaled YAML to contain the path, got:\n%s", s)
	}
	if !strings.Contains(s, "openapi:") {
		t.Errorf("expected marshaled YAML to contain the openapi version field, got:\n%s", s)
	}
}
