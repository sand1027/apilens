package assertions

import (
	"database/sql"
	"testing"

	"github.com/sandeepv/apilens/internal/dbassert"
	"github.com/sandeepv/apilens/internal/domain"
)

// seedRegistryDB creates and populates a "users" table via a SEPARATE
// connection using the same shared-cache in-memory DSN the registry uses
// ("file:...?mode=memory&cache=shared") — sqlite's shared-cache mode is
// what lets this connection and the registry's own lazily-opened
// connection see the same in-memory database (a plain ":memory:" DSN
// would give each connection its own private, empty database).
// dbassert.Registry deliberately has no exported "raw *sql.DB" accessor
// (checks should only ever go through RowCount/FirstValue), so tests seed
// data this way instead.
func seedRegistryDB(t *testing.T, reg *dbassert.Registry, connName string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:apilens_dbcheck_test?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS users (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	if _, err := db.Exec(`DELETE FROM users`); err != nil {
		t.Fatalf("clearing table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, name) VALUES (1, 'ada')`); err != nil {
		t.Fatalf("seeding row: %v", err)
	}
}

func newSharedMemoryRegistry(t *testing.T) *dbassert.Registry {
	t.Helper()
	reg := dbassert.NewRegistry(map[string]dbassert.ConnectionConfig{
		"main": {Driver: "sqlite", DSN: "file:apilens_dbcheck_test?mode=memory&cache=shared"},
	})
	t.Cleanup(reg.Close)
	seedRegistryDB(t, reg, "main")
	return reg
}

func exchangeIgnoringBody() domain.Exchange {
	return domain.Exchange{Response: domain.HTTPResponse{StatusCode: 200}}
}

func TestEngine_DBRowCountEquals_Passes(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	want := 1
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT * FROM users WHERE id = 1", RowCountEquals: &want},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if !results[0].Passed {
		t.Errorf("expected db.main row_count_equals to pass, got %+v", results[0])
	}
}

func TestEngine_DBRowCountEquals_Fails(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	want := 5
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT * FROM users", RowCountEquals: &want},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if results[0].Passed {
		t.Error("expected db.main row_count_equals to fail (expected 5, actual 1)")
	}
}

func TestEngine_DBExists_TrueAndFalse(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	yes := true
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT * FROM users WHERE id = 1", Exists: &yes},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if !results[0].Passed {
		t.Errorf("expected exists=true to pass for a matching row, got %+v", results[0])
	}

	set2, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT * FROM users WHERE id = 999", Exists: &yes},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results2 := e.Eval(set2, exchangeIgnoringBody())
	if results2[0].Passed {
		t.Error("expected exists=true to fail for a non-matching row")
	}
}

func TestEngine_DBEquals_ComparesScalarValue(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT name FROM users WHERE id = 1", Equals: "ada"},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if !results[0].Passed {
		t.Errorf("expected db.main equals to pass, got %+v", results[0])
	}
}

func TestEngine_DBAssertion_RejectsMutatingQueryAtCompileTime(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	want := 1
	_, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "DELETE FROM users", RowCountEquals: &want},
		},
	})
	if err == nil {
		t.Fatal("expected a mutating query to be rejected at Compile time, before any Eval")
	}
}

func TestEngine_DBAssertion_WithoutRegistryIsConfigError(t *testing.T) {
	e := New() // no WithDBRegistry
	want := 1
	_, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT 1", RowCountEquals: &want},
		},
	})
	if err == nil {
		t.Fatal("expected a config error when db.* is used with no registry configured")
	}
}

func TestEngine_DBAssertion_QueryErrorSurfacesAsAssertionFailureNotPanic(t *testing.T) {
	reg := newSharedMemoryRegistry(t)
	e := New(WithDBRegistry(reg))
	want := 1
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string]domain.DBSpec{
			"main": {Query: "SELECT * FROM nonexistent_table", RowCountEquals: &want},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if results[0].Passed {
		t.Error("expected a query against a nonexistent table to fail, not pass")
	}
	if results[0].Reason == "" {
		t.Error("expected a non-empty failure reason")
	}
}
