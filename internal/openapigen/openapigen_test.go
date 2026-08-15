package openapigen

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func loadSpec(t *testing.T, raw string) *openapi3.T {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData([]byte(raw))
	if err != nil {
		t.Fatalf("LoadFromData: %v", err)
	}
	if err := doc.Validate(context.Background()); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	return doc
}

const specWithExamples = `
openapi: 3.0.3
info:
  title: Test
  version: "1.0"
paths:
  /health:
    get:
      operationId: health
      responses:
        "200":
          description: OK
  /api/users:
    get:
      operationId: listUsers
      tags: [users]
      responses:
        "200":
          description: A list of users
    post:
      operationId: createUser
      tags: [users]
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
            example:
              name: Ada Lovelace
              email: ada@example.com
      responses:
        "201":
          description: Created
  /api/users/{id}:
    get:
      operationId: getUser
      tags: [users]
      parameters:
        - name: id
          in: path
          required: true
          schema:
            type: integer
          example: 42
      responses:
        "200":
          description: A single user
        "404":
          description: Not found
  /api/orders/{orderId}:
    get:
      operationId: getOrderNoExample
      parameters:
        - name: orderId
          in: path
          required: true
          schema:
            type: string
      responses:
        "200":
          description: An order
  /api/search:
    get:
      operationId: search
      parameters:
        - name: q
          in: query
          schema:
            type: string
          example: apples
      responses:
        "200":
          description: Search results
`

func TestFromDoc_GeneratesOneTestPerOperationWithUsableExamples(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	generated := FromDoc(doc)

	ids := map[string]bool{}
	for _, g := range generated {
		ids[g.OperationID] = true
	}
	for _, want := range []string{"health", "listUsers", "createUser", "getUser", "search"} {
		if !ids[want] {
			t.Errorf("expected operation %q to be generated, got %v", want, ids)
		}
	}
	if ids["getOrderNoExample"] {
		t.Error("expected getOrderNoExample (no path param example) to be skipped, not generated")
	}
	if len(generated) != 5 {
		t.Errorf("expected exactly 5 generated tests, got %d", len(generated))
	}
}

func TestFromDoc_FillsPathParamFromExample(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	generated := FromDoc(doc)
	g := findGenerated(t, generated, "getUser")
	want := "{{base_url}}/api/users/42"
	if g.Test.Request.URL != want {
		t.Errorf("URL = %q, want %q", g.Test.Request.URL, want)
	}
}

func TestFromDoc_FillsRequestBodyFromExample(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	generated := FromDoc(doc)
	g := findGenerated(t, generated, "createUser")
	body, ok := g.Test.Request.Body.JSON.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON body map, got %T", g.Test.Request.Body.JSON)
	}
	if body["name"] != "Ada Lovelace" || body["email"] != "ada@example.com" {
		t.Errorf("body = %+v", body)
	}
}

func TestFromDoc_AssertsLowestDeclaredSuccessStatus(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	generated := FromDoc(doc)

	create := findGenerated(t, generated, "createUser")
	if *create.Test.Assert.Status.Equals != 201 {
		t.Errorf("createUser status = %d, want 201", *create.Test.Assert.Status.Equals)
	}
	health := findGenerated(t, generated, "health")
	if *health.Test.Assert.Status.Equals != 200 {
		t.Errorf("health status = %d, want 200", *health.Test.Assert.Status.Equals)
	}
}

func TestFromDoc_FillsQueryParamFromExample(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	generated := FromDoc(doc)
	g := findGenerated(t, generated, "search")
	if g.Test.Request.Query["q"] != "apples" {
		t.Errorf("query[q] = %q, want %q", g.Test.Request.Query["q"], "apples")
	}
}

func TestFromDoc_SkipsOperationWithRequiredBodyAndNoExample(t *testing.T) {
	spec := `
openapi: 3.0.3
info: {title: t, version: "1.0"}
paths:
  /api/things:
    post:
      operationId: createThing
      requestBody:
        required: true
        content:
          application/json:
            schema:
              type: object
      responses:
        "201":
          description: Created
`
	doc := loadSpec(t, spec)
	generated := FromDoc(doc)
	for _, g := range generated {
		if g.OperationID == "createThing" {
			t.Fatal("expected createThing (required body, no example) to be skipped")
		}
	}
}

func TestFromDoc_OptionalBodyWithNoExampleStillGenerates(t *testing.T) {
	spec := `
openapi: 3.0.3
info: {title: t, version: "1.0"}
paths:
  /api/things:
    post:
      operationId: createThing
      requestBody:
        required: false
        content:
          application/json:
            schema:
              type: object
      responses:
        "201":
          description: Created
`
	doc := loadSpec(t, spec)
	generated := FromDoc(doc)
	found := false
	for _, g := range generated {
		if g.OperationID == "createThing" {
			found = true
			if g.Test.Request.Body.JSON != nil {
				t.Errorf("expected no body to be filled, got %v", g.Test.Request.Body.JSON)
			}
		}
	}
	if !found {
		t.Fatal("expected createThing (optional body, no example) to still be generated")
	}
}

func TestFromDoc_NilDocReturnsNil(t *testing.T) {
	if got := FromDoc(nil); got != nil {
		t.Errorf("expected nil for a nil doc, got %v", got)
	}
}

func TestFromDoc_DeterministicOrderAcrossRuns(t *testing.T) {
	doc := loadSpec(t, specWithExamples)
	first := FromDoc(doc)
	second := FromDoc(doc)
	if len(first) != len(second) {
		t.Fatalf("length mismatch: %d vs %d", len(first), len(second))
	}
	for i := range first {
		if first[i].OperationID != second[i].OperationID {
			t.Errorf("order mismatch at %d: %q vs %q", i, first[i].OperationID, second[i].OperationID)
		}
	}
}

func findGenerated(t *testing.T, generated []Generated, opID string) Generated {
	t.Helper()
	for _, g := range generated {
		if g.OperationID == opID {
			return g
		}
	}
	t.Fatalf("no generated test for operation %q", opID)
	return Generated{}
}
