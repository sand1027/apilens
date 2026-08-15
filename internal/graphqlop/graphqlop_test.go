package graphqlop

import (
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestLooksLikeAndParseHTTPBody(t *testing.T) {
	body := []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`)
	if !LooksLike(body) {
		t.Fatal("expected GraphQL payload")
	}
	p, op, ok := ParseHTTPBody(body)
	if !ok {
		t.Fatal("ParseHTTPBody failed")
	}
	if p.OperationName != "Ping" {
		t.Errorf("OperationName = %q", p.OperationName)
	}
	if op.Type != TypeQuery {
		t.Errorf("Type = %q", op.Type)
	}
	if op.PrimaryField() != "ping" {
		t.Errorf("PrimaryField = %q", op.PrimaryField())
	}
	if RegistryPath(op.Type, op.PrimaryField()) != "/graphql/query/ping" {
		t.Errorf("path = %s", RegistryPath(op.Type, op.PrimaryField()))
	}
}

func TestParseMutation(t *testing.T) {
	op, err := ParseQuery(`mutation Pong($msg: String!) { pong(input: {message: $msg}) { message } }`)
	if err != nil {
		t.Fatal(err)
	}
	if op.Type != TypeMutation || op.Name != "Pong" || op.PrimaryField() != "pong" {
		t.Fatalf("got %+v", op)
	}
	if DisplayMethod(op.Type) != "MUTATION" {
		t.Errorf("DisplayMethod = %s", DisplayMethod(op.Type))
	}
}

func TestHTTPMethodFor(t *testing.T) {
	if HTTPMethodFor("QUERY") != "POST" {
		t.Fatal("QUERY should send POST")
	}
	if HTTPMethodFor("GET") != "GET" {
		t.Fatal("GET should stay GET")
	}
	if HTTPMethodFor(domain.Method("mutation")) != "POST" {
		t.Fatal("mutation should send POST")
	}
}

func TestLooksLikeRejectsREST(t *testing.T) {
	if LooksLike([]byte(`{"name":"ada"}`)) {
		t.Fatal("REST JSON should not look like GraphQL")
	}
	if LooksLike([]byte(`not json`)) {
		t.Fatal("garbage should not look like GraphQL")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	raw, err := Encode("query { ping { message } }", map[string]any{"x": 1}, "Ping")
	if err != nil {
		t.Fatal(err)
	}
	p, ok := Decode(raw)
	if !ok || p.OperationName != "Ping" || p.Variables["x"].(float64) != 1 {
		t.Fatalf("round trip failed: %+v", p)
	}
}

func TestDisplayColumns_GraphQLUsesOperationNotHTTPURL(t *testing.T) {
	body := []byte(`{"query":"query Ping { ping { message } }","operationName":"Ping"}`)
	method, name := DisplayColumns("POST", "http://localhost:3000/graphql", body)
	if method != "QUERY" || name != "Ping" {
		t.Fatalf("got %s %s, want QUERY Ping", method, name)
	}
}

func TestDisplayColumns_RESTKeepsURL(t *testing.T) {
	method, url := DisplayColumns("GET", "http://localhost:3001/login", nil)
	if method != "GET" || url != "http://localhost:3001/login" {
		t.Fatalf("got %s %s", method, url)
	}
}

func TestResponseHasErrors(t *testing.T) {
	if ResponseHasErrors([]byte(`{"data":{"ping":{}}}`)) {
		t.Fatal("successful envelope must not report errors")
	}
	if !ResponseHasErrors([]byte(`{"data":null,"errors":[{"message":"Not Authorised!"}]}`)) {
		t.Fatal("errors[] must be detected")
	}
}
