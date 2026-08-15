package dbassert

import (
	"context"
	"os"
	"testing"
)

func newSQLiteRegistry(t *testing.T) *Registry {
	t.Helper()
	reg := NewRegistry(map[string]ConnectionConfig{
		"test": {Driver: "sqlite", DSN: ":memory:"},
	})
	t.Cleanup(reg.Close)

	db, err := reg.get("test")
	if err != nil {
		t.Fatalf("opening test db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT, email TEXT)`); err != nil {
		t.Fatalf("creating table: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (id, name, email) VALUES (1, 'ada', 'ada@example.com')`); err != nil {
		t.Fatalf("seeding row: %v", err)
	}
	return reg
}

func TestValidateReadOnly_AcceptsSelect(t *testing.T) {
	if err := ValidateReadOnly("SELECT * FROM users"); err != nil {
		t.Errorf("expected a SELECT to be accepted, got %v", err)
	}
}

func TestValidateReadOnly_RejectsMutatingKeywords(t *testing.T) {
	for _, q := range []string{
		"DELETE FROM users",
		"UPDATE users SET name = 'x'",
		"INSERT INTO users VALUES (1)",
		"DROP TABLE users",
		"SELECT * FROM users; DELETE FROM users",
	} {
		if err := ValidateReadOnly(q); err == nil {
			t.Errorf("expected %q to be rejected as non-read-only", q)
		}
	}
}

func TestValidateReadOnly_RejectsEmptyQuery(t *testing.T) {
	if err := ValidateReadOnly("   "); err == nil {
		t.Error("expected an empty query to be rejected")
	}
}

func TestValidateReadOnly_RejectsNonSelectStatement(t *testing.T) {
	if err := ValidateReadOnly("PRAGMA table_info(users)"); err == nil {
		t.Error("expected a non-SELECT statement to be rejected")
	}
}

func TestValidateReadOnly_AllowsColumnNamedLikeAKeyword(t *testing.T) {
	// "updated_at" contains "update" as a substring but not as a whole
	// word — must not false-positive.
	if err := ValidateReadOnly("SELECT updated_at FROM users"); err != nil {
		t.Errorf("expected a column named like a keyword to be accepted, got %v", err)
	}
}

func TestRowCount_ReturnsActualRowCount(t *testing.T) {
	reg := newSQLiteRegistry(t)
	count, err := reg.RowCount(context.Background(), "test", "SELECT * FROM users WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("RowCount: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestRowCount_ZeroForNoMatch(t *testing.T) {
	reg := newSQLiteRegistry(t)
	count, err := reg.RowCount(context.Background(), "test", "SELECT * FROM users WHERE id = ?", 999)
	if err != nil {
		t.Fatalf("RowCount: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestFirstValue_ReturnsScalarColumn(t *testing.T) {
	reg := newSQLiteRegistry(t)
	val, found, err := reg.FirstValue(context.Background(), "test", "SELECT name FROM users WHERE id = ?", 1)
	if err != nil {
		t.Fatalf("FirstValue: %v", err)
	}
	if !found {
		t.Fatal("expected a row to be found")
	}
	if val != "ada" {
		t.Errorf("val = %q, want %q", val, "ada")
	}
}

func TestFirstValue_NotFoundForNoRows(t *testing.T) {
	reg := newSQLiteRegistry(t)
	_, found, err := reg.FirstValue(context.Background(), "test", "SELECT name FROM users WHERE id = ?", 999)
	if err != nil {
		t.Fatalf("FirstValue: %v", err)
	}
	if found {
		t.Error("expected found=false for a query with no matching rows")
	}
}

func TestGet_UnconfiguredConnectionIsError(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.RowCount(context.Background(), "does-not-exist", "SELECT 1")
	if err == nil {
		t.Fatal("expected an error for an unconfigured connection name")
	}
}

func TestGet_UnsupportedDriverIsError(t *testing.T) {
	reg := NewRegistry(map[string]ConnectionConfig{"bad": {Driver: "oracle", DSN: "x"}})
	_, err := reg.RowCount(context.Background(), "bad", "SELECT 1")
	if err == nil {
		t.Fatal("expected an error for an unsupported driver")
	}
}

func TestExpandDSN_ExpandsEnvVariable(t *testing.T) {
	t.Setenv("APILENS_TEST_DSN_VALUE", "resolved-value")
	got, err := expandDSN("prefix-${APILENS_TEST_DSN_VALUE}-suffix")
	if err != nil {
		t.Fatalf("expandDSN: %v", err)
	}
	if got != "prefix-resolved-value-suffix" {
		t.Errorf("got %q", got)
	}
}

func TestExpandDSN_MissingVariableFailsClosed(t *testing.T) {
	_ = os.Unsetenv("APILENS_TEST_DSN_DEFINITELY_UNSET")
	_, err := expandDSN("${APILENS_TEST_DSN_DEFINITELY_UNSET}")
	if err == nil {
		t.Fatal("expected an error for a missing DSN environment variable")
	}
}

func TestGet_CachesConnectionAcrossCalls(t *testing.T) {
	reg := newSQLiteRegistry(t)
	db1, err := reg.get("test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	db2, err := reg.get("test")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if db1 != db2 {
		t.Error("expected the same *sql.DB instance to be returned on repeated get calls")
	}
}
