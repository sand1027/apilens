package testdef

import (
	"os"
	"testing"
)

func loadFixture(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading fixture %s: %v", path, err)
	}
	return data
}

func TestCompile_MinimalValidTest(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/valid/minimal.yaml")
	tc, err := Compile(raw, "minimal.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if tc.Name != "Health" {
		t.Errorf("Name = %q", tc.Name)
	}
	if tc.Request.Method != "GET" {
		t.Errorf("Method = %q", tc.Request.Method)
	}
	if tc.Assert.Status == nil || tc.Assert.Status.Equals == nil || *tc.Assert.Status.Equals != 200 {
		t.Errorf("Assert.Status = %+v", tc.Assert.Status)
	}
	if tc.Timeout != DefaultTimeout {
		t.Errorf("Timeout = %v, want default %v", tc.Timeout, DefaultTimeout)
	}
	if tc.Retries != DefaultRetries {
		t.Errorf("Retries = %d, want default %d", tc.Retries, DefaultRetries)
	}
}

func TestCompile_FullValidTest(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/valid/full.yaml")
	tc, err := Compile(raw, "full.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if len(tc.Tags) != 2 {
		t.Errorf("Tags = %v", tc.Tags)
	}
	if tc.Request.Auth == nil || tc.Request.Auth.Type != "bearer" {
		t.Errorf("Auth = %+v", tc.Request.Auth)
	}
	if len(tc.Assert.Headers) != 2 {
		t.Errorf("Assert.Headers = %+v", tc.Assert.Headers)
	}
	if len(tc.Assert.JSON) != 4 {
		t.Errorf("Assert.JSON = %+v", tc.Assert.JSON)
	}
}

func TestCompile_MissingNameFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/missing-name.yaml")
	_, err := Compile(raw, "missing-name.yaml")
	if err == nil {
		t.Fatal("expected error for missing name")
	}
}

func TestCompile_NoAssertionsFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/no-assertions.yaml")
	_, err := Compile(raw, "no-assertions.yaml")
	if err == nil {
		t.Fatal("expected error for test with no assertions")
	}
}

func TestCompile_BothBodyFormsFails(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/invalid/both-body-forms.yaml")
	_, err := Compile(raw, "both-body-forms.yaml")
	if err == nil {
		t.Fatal("expected error for both json and raw body")
	}
}

func TestCompile_BadYAMLFails(t *testing.T) {
	_, err := Compile([]byte("not: valid: yaml: at: all:"), "bad.yaml")
	if err == nil {
		t.Fatal("expected error for malformed YAML")
	}
}

func TestCompile_InvalidTimeoutFails(t *testing.T) {
	raw := []byte(`
name: Test
request:
  method: GET
  url: "{{base_url}}/x"
  timeout: not-a-duration
assert:
  status:
    equals: 200
`)
	_, err := Compile(raw, "bad-timeout.yaml")
	if err == nil {
		t.Fatal("expected error for invalid timeout")
	}
}

func TestCompile_GraphQLTest(t *testing.T) {
	raw := loadFixture(t, "../../testdata/tests/valid/graphql.yaml")
	tc, err := Compile(raw, "graphql.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	if tc.Request.Method != "POST" {
		t.Errorf("Method = %q, want POST", tc.Request.Method)
	}
	if tc.Request.URL != "{{base_url}}/graphql" {
		t.Errorf("URL = %q", tc.Request.URL)
	}
	if tc.Request.GraphQL == nil || tc.Request.GraphQL.OperationName != "Ping" {
		t.Fatalf("GraphQL = %+v", tc.Request.GraphQL)
	}
	if tc.Assert.GraphQL == nil || tc.Assert.GraphQL.NoErrors == nil || !*tc.Assert.GraphQL.NoErrors {
		t.Errorf("Assert.GraphQL = %+v", tc.Assert.GraphQL)
	}
}

func TestCompile_GraphQLCannotMixBody(t *testing.T) {
	raw := []byte(`
name: Bad
request:
  graphql:
    query: "{ ping { message } }"
  body:
    json: { "x": 1 }
assert:
  graphql:
    no_errors: true
`)
	_, err := Compile(raw, "bad.yaml")
	if err == nil {
		t.Fatal("expected error combining graphql and body")
	}
}

func TestCompile_DBAssertionSQLShapeCompiles(t *testing.T) {
	raw := []byte(`
name: SQL db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  status:
    equals: 200
  db:
    main:
      query: "SELECT * FROM users WHERE id = 1"
      exists: true
`)
	tc, err := Compile(raw, "db-sql.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	specs, ok := tc.Assert.DB["main"]
	if !ok || len(specs) != 1 {
		t.Fatal("expected exactly one db.main check in compiled assertion spec")
	}
	spec := specs[0]
	if spec.Query == "" || spec.Collection != "" {
		t.Errorf("spec = %+v, want SQL shape (Query set, Collection empty)", spec)
	}
}

func TestCompile_DBAssertionMongoShapeCompiles(t *testing.T) {
	raw := []byte(`
name: Mongo db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  status:
    equals: 200
  db:
    main:
      collection: users
      filter:
        email: ada@example.com
      exists: true
`)
	tc, err := Compile(raw, "db-mongo.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	specs, ok := tc.Assert.DB["main"]
	if !ok || len(specs) != 1 {
		t.Fatal("expected exactly one db.main check in compiled assertion spec")
	}
	spec := specs[0]
	if spec.Collection != "users" || spec.Query != "" {
		t.Errorf("spec = %+v, want MongoDB shape (Collection set, Query empty)", spec)
	}
	filter, ok := spec.Filter.(map[string]any)
	if !ok || filter["email"] != "ada@example.com" {
		t.Errorf("Filter = %#v", spec.Filter)
	}
}

func TestCompile_DBAssertionAcceptsListOfChecksPerConnection(t *testing.T) {
	raw := []byte(`
name: Advance mutation wrote everywhere expected
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  status:
    equals: 200
  db:
    main:
      - collection: advances
        filter: { _id: "abc123" }
        exists: true
      - collection: receipts
        filter: { _id: "def456" }
        exists: true
`)
	tc, err := Compile(raw, "db-list.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	specs, ok := tc.Assert.DB["main"]
	if !ok {
		t.Fatal("expected db.main in compiled assertion spec")
	}
	if len(specs) != 2 {
		t.Fatalf("expected 2 checks under db.main, got %d", len(specs))
	}
	if specs[0].Collection != "advances" {
		t.Errorf("specs[0].Collection = %q, want advances", specs[0].Collection)
	}
	if specs[1].Collection != "receipts" {
		t.Errorf("specs[1].Collection = %q, want receipts", specs[1].Collection)
	}
}

func TestCompile_DBAssertionSingleMappingStillWorks(t *testing.T) {
	// Backward compatibility: a bare mapping (the original v9 shape, one
	// check per connection) must still normalize to a one-element slice
	// rather than requiring every existing test file to be rewritten as
	// a list.
	raw := []byte(`
name: Old-style single db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  db:
    main:
      collection: centers
      filter: {}
      exists: true
`)
	tc, err := Compile(raw, "db-single.yaml")
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	specs, ok := tc.Assert.DB["main"]
	if !ok || len(specs) != 1 {
		t.Fatalf("expected exactly one check under db.main, got %v", specs)
	}
	if specs[0].Collection != "centers" {
		t.Errorf("Collection = %q, want centers", specs[0].Collection)
	}
}

func TestCompile_DBAssertionEmptyListIsAnError(t *testing.T) {
	raw := []byte(`
name: Bad db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  db:
    main: []
`)
	_, err := Compile(raw, "bad-db-empty.yaml")
	if err == nil {
		t.Fatal("expected an error when db.<connection> is an empty list")
	}
}

func TestCompile_DBAssertionListEntryStillEnforcesQueryCollectionMutex(t *testing.T) {
	raw := []byte(`
name: Bad db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  db:
    main:
      - collection: advances
        filter: {}
        exists: true
      - query: "SELECT 1"
        collection: receipts
        exists: true
`)
	_, err := Compile(raw, "bad-db-list.yaml")
	if err == nil {
		t.Fatal("expected an error when one list entry sets both query and collection")
	}
}

func TestCompile_DBAssertionCannotSetBothQueryAndCollection(t *testing.T) {
	raw := []byte(`
name: Bad db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  db:
    main:
      query: "SELECT 1"
      collection: users
      exists: true
`)
	_, err := Compile(raw, "bad-db.yaml")
	if err == nil {
		t.Fatal("expected an error when both query and collection are set on the same db.<connection> block")
	}
}

func TestCompile_DBAssertionRequiresQueryOrCollection(t *testing.T) {
	raw := []byte(`
name: Bad db check
request:
  method: GET
  url: "{{base_url}}/health"
assert:
  db:
    main:
      exists: true
`)
	_, err := Compile(raw, "bad-db.yaml")
	if err == nil {
		t.Fatal("expected an error when neither query nor collection is set")
	}
}
