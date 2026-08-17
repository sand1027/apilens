package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func writeCoverageTest(t *testing.T, dir, name, content string) {
	t.Helper()
	testsDir := filepath.Join(dir, ".apilens", "tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatalf("mkdir tests dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, name), []byte(content), 0o644); err != nil {
		t.Fatalf("writing test file: %v", err)
	}
}

func TestCoverage_RESTEndpointMatchedByMethodAndPath(t *testing.T) {
	a, dir := newTestApp(t) // fixture has GET/POST /api/users and GET /api/users/{id}
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	writeCoverageTest(t, dir, "list-users.yaml", `
version: 1
name: List users
request:
  method: GET
  url: "{{base_url}}/api/users"
assert:
  status:
    equals: 200
`)

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if res.Total != 3 {
		t.Fatalf("expected 3 discovered endpoints, got %d", res.Total)
	}
	if len(res.Covered) != 1 {
		t.Fatalf("expected exactly 1 covered endpoint, got %d: %+v", len(res.Covered), res.Covered)
	}
	covered := res.Covered[0]
	if covered.Method != "GET" || covered.Path != "/api/users" {
		t.Errorf("covered endpoint = %+v, want GET /api/users", covered)
	}
	if len(covered.Tests) != 1 || covered.Tests[0] != "List users" {
		t.Errorf("Tests = %v, want [List users]", covered.Tests)
	}
	if len(res.Missing) != 2 {
		t.Fatalf("expected 2 uncovered endpoints, got %d: %+v", len(res.Missing), res.Missing)
	}
}

func TestCoverage_PathParamMatchesNormalizedRegistryEntry(t *testing.T) {
	// The registry stores /api/users/{id}; NormalizePath canonicalizes
	// that to /api/users/:id. A test hitting a REAL id (e.g. /api/users/1)
	// must resolve to the SAME EndpointID, since NewEndpointID always
	// normalizes both sides before hashing.
	a, dir := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	writeCoverageTest(t, dir, "get-user.yaml", `
version: 1
name: Get one user
request:
  method: GET
  url: "{{base_url}}/api/users/1"
assert:
  status:
    equals: 200
`)

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	// Discovery already normalizes {id} -> :id when the endpoint is
	// stored in the registry (domain.NormalizePath), so the covered
	// entry's Path reflects that canonical form, not the raw OpenAPI
	// {id} spelling.
	var found bool
	for _, ep := range res.Covered {
		if ep.Method == "GET" && ep.Path == "/api/users/:id" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected GET /api/users/:id to be covered, got covered=%+v missing=%+v", res.Covered, res.Missing)
	}
}

func TestCoverage_MultipleTestsOnSameEndpointAllListed(t *testing.T) {
	a, dir := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	writeCoverageTest(t, dir, "list-users-a.yaml", `
version: 1
name: List users happy path
request:
  method: GET
  url: "{{base_url}}/api/users"
assert:
  status:
    equals: 200
`)
	writeCoverageTest(t, dir, "list-users-b.yaml", `
version: 1
name: List users again
request:
  method: GET
  url: "{{base_url}}/api/users"
assert:
  status:
    equals: 200
`)

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(res.Covered) != 1 {
		t.Fatalf("expected 1 covered endpoint (both tests hit the same one), got %d", len(res.Covered))
	}
	if len(res.Covered[0].Tests) != 2 {
		t.Errorf("expected both test names listed, got %v", res.Covered[0].Tests)
	}
}

func TestCoverage_NoTestsMeansEverythingUncovered(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(res.Covered) != 0 {
		t.Errorf("expected 0 covered with no test files, got %d", len(res.Covered))
	}
	if len(res.Missing) != 3 {
		t.Errorf("expected all 3 discovered endpoints uncovered, got %d", len(res.Missing))
	}
	if res.Percent() != 0 {
		t.Errorf("Percent() = %v, want 0", res.Percent())
	}
}

func TestCoverage_NoRegistryAtAllReturnsZeroTotal(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if res.Total != 0 {
		t.Errorf("Total = %d, want 0 (never discovered)", res.Total)
	}
	if res.Percent() != 0 {
		t.Errorf("Percent() = %v, want 0 for a zero-endpoint registry", res.Percent())
	}
}

func TestCoverage_UnresolvableChainedURLIsExcludedNotErrored(t *testing.T) {
	// A test whose URL still contains a "{{responses...}}" placeholder
	// (DSL v2 chaining) cannot be resolved to a static path without
	// actually running the suite -- Coverage must skip it rather than
	// erroring the whole command.
	a, dir := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	writeCoverageTest(t, dir, "chained.yaml", `
version: 2
id: get_created_user
name: Get the just-created user
request:
  method: GET
  url: "{{base_url}}/api/users/{{responses.create_user.body.data.id}}"
assert:
  status:
    equals: 200
`)

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v (must not error on an unresolvable chained URL)", err)
	}
	if len(res.Covered) != 0 {
		t.Errorf("expected the chained test to NOT count toward coverage, got %+v", res.Covered)
	}
}

func TestCoverage_GraphQLTestMatchesDiscoveredOperation(t *testing.T) {
	dir := t.TempDir()
	schema := `
type Query {
  ping: String!
}
type Mutation {
  createAdvance(total: Float!): String!
}
`
	if err := os.WriteFile(filepath.Join(dir, "schema.graphql"), []byte(schema), 0o644); err != nil {
		t.Fatalf("writing schema fixture: %v", err)
	}
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}

	writeCoverageTest(t, dir, "create-advance.yaml", `
version: 1
name: Create advance mutation
request:
  graphql:
    query: |
      mutation CreateAdvance($total: Float!) {
        createAdvance(total: $total)
      }
    variables:
      total: 100
assert:
  status:
    equals: 200
`)

	res, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	var sawCreateAdvance, sawPing bool
	for _, ep := range res.Covered {
		if ep.Path == "/graphql/mutation/createAdvance" {
			sawCreateAdvance = true
		}
	}
	for _, ep := range res.Missing {
		if ep.Path == "/graphql/query/ping" {
			sawPing = true
		}
	}
	if !sawCreateAdvance {
		t.Errorf("expected createAdvance to be covered, got covered=%+v", res.Covered)
	}
	if !sawPing {
		t.Errorf("expected ping to remain uncovered, got missing=%+v", res.Missing)
	}
}

func TestCoverage_ResultsAreSortedDeterministically(t *testing.T) {
	a, dir := newTestApp(t)
	if _, err := a.Discover(context.Background(), DiscoverOptions{}); err != nil {
		t.Fatalf("Discover: %v", err)
	}
	writeCoverageTest(t, dir, "t.yaml", `
version: 1
name: t
request:
  method: GET
  url: "{{base_url}}/api/users"
assert:
  status:
    equals: 200
`)

	first, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	second, err := a.Coverage()
	if err != nil {
		t.Fatalf("Coverage: %v", err)
	}
	if len(first.Missing) != len(second.Missing) {
		t.Fatal("expected deterministic Missing length across repeated calls")
	}
	for i := range first.Missing {
		if first.Missing[i].Method != second.Missing[i].Method || first.Missing[i].Path != second.Missing[i].Path {
			t.Errorf("Missing[%d] differs across calls: %+v vs %+v", i, first.Missing[i], second.Missing[i])
		}
	}
}

var _ = domain.TestCase{} // keep domain imported for future assertions if needed
