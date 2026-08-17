package dbassert

import (
	"context"
	"os"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoTestDSN points at a local MongoDB instance for these tests. Set
// APILENS_TEST_MONGO_DSN to override (e.g. in CI); defaults to the
// database used for manual local testing against a real `mongod`.
func mongoTestDSN() string {
	if v := os.Getenv("APILENS_TEST_MONGO_DSN"); v != "" {
		return v
	}
	return "mongodb://127.0.0.1:27017/apilens_dbassert_test"
}

// skipIfNoMongo pings a fresh client and skips the test if no MongoDB
// server is reachable, so `go test ./...` still passes in environments
// without a local mongod (CI images, sandboxes) — mirroring how this
// package's SQL tests need no such guard (sqlite is always available,
// pure Go, no external server).
func skipIfNoMongo(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(mongoTestDSN()).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("no MongoDB available for integration test: %v", err)
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("no MongoDB available for integration test: %v", err)
	}
}

// seedMongoRegistry builds a Registry pointed at mongoTestDSN, seeding
// (and clearing beforehand) a "users" collection with one document.
func seedMongoRegistry(t *testing.T) *Registry {
	t.Helper()
	skipIfNoMongo(t)

	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)

	db, err := reg.getMongo("main")
	if err != nil {
		t.Fatalf("getMongo: %v", err)
	}
	coll := db.Collection("users")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := coll.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clearing collection: %v", err)
	}
	if _, err := coll.InsertOne(ctx, bson.M{"_id": "u1", "name": "ada", "email": "ada@example.com", "age": 30}); err != nil {
		t.Fatalf("seeding document: %v", err)
	}
	return reg
}

func TestRowCountMongo_ReturnsActualCount(t *testing.T) {
	reg := seedMongoRegistry(t)
	count, err := reg.RowCountMongo(context.Background(), "main", "users", map[string]any{"email": "ada@example.com"})
	if err != nil {
		t.Fatalf("RowCountMongo: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestRowCountMongo_ZeroForNoMatch(t *testing.T) {
	reg := seedMongoRegistry(t)
	count, err := reg.RowCountMongo(context.Background(), "main", "users", map[string]any{"email": "nobody@example.com"})
	if err != nil {
		t.Fatalf("RowCountMongo: %v", err)
	}
	if count != 0 {
		t.Errorf("count = %d, want 0", count)
	}
}

func TestRowCountMongo_NilFilterMatchesEverything(t *testing.T) {
	reg := seedMongoRegistry(t)
	count, err := reg.RowCountMongo(context.Background(), "main", "users", nil)
	if err != nil {
		t.Fatalf("RowCountMongo: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1", count)
	}
}

func TestFirstValueMongo_ReturnsNamedField(t *testing.T) {
	reg := seedMongoRegistry(t)
	val, found, err := reg.FirstValueMongo(context.Background(), "main", "users", map[string]any{"email": "ada@example.com"}, "name")
	if err != nil {
		t.Fatalf("FirstValueMongo: %v", err)
	}
	if !found {
		t.Fatal("expected a document to be found")
	}
	if val != "ada" {
		t.Errorf("val = %q, want %q", val, "ada")
	}
}

func TestFirstValueMongo_DefaultsFieldToID(t *testing.T) {
	reg := seedMongoRegistry(t)
	val, found, err := reg.FirstValueMongo(context.Background(), "main", "users", map[string]any{"email": "ada@example.com"}, "")
	if err != nil {
		t.Fatalf("FirstValueMongo: %v", err)
	}
	if !found {
		t.Fatal("expected a document to be found")
	}
	if val != "u1" {
		t.Errorf("val = %q, want %q (default field should be _id)", val, "u1")
	}
}

func TestFirstValueMongo_NotFoundForNoMatch(t *testing.T) {
	reg := seedMongoRegistry(t)
	_, found, err := reg.FirstValueMongo(context.Background(), "main", "users", map[string]any{"email": "nobody@example.com"}, "name")
	if err != nil {
		t.Fatalf("FirstValueMongo: %v", err)
	}
	if found {
		t.Error("expected found=false for a filter with no matching documents")
	}
}

func TestFirstValueMongo_NumericFieldFormatsAsPlainNumber(t *testing.T) {
	reg := seedMongoRegistry(t)
	val, found, err := reg.FirstValueMongo(context.Background(), "main", "users", map[string]any{"email": "ada@example.com"}, "age")
	if err != nil {
		t.Fatalf("FirstValueMongo: %v", err)
	}
	if !found {
		t.Fatal("expected a document to be found")
	}
	if val != "30" {
		t.Errorf("val = %q, want %q", val, "30")
	}
}

func TestGetMongo_ConnectionConfiguredAsSQLDriverIsError(t *testing.T) {
	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "sqlite", DSN: ":memory:"},
	})
	t.Cleanup(reg.Close)
	_, err := reg.RowCountMongo(context.Background(), "main", "users", nil)
	if err == nil {
		t.Fatal("expected an error when a sqlite-configured connection is used for a MongoDB check")
	}
}

func TestGet_ConnectionConfiguredAsMongoIsErrorForSQLCheck(t *testing.T) {
	skipIfNoMongo(t)
	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)
	_, err := reg.RowCount(context.Background(), "main", "SELECT 1")
	if err == nil {
		t.Fatal("expected an error when a mongodb-configured connection is used for a SQL query")
	}
}

func TestGetMongo_UnconfiguredConnectionIsError(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.RowCountMongo(context.Background(), "does-not-exist", "users", nil)
	if err == nil {
		t.Fatal("expected an error for an unconfigured connection name")
	}
}

func TestGetMongo_DSNWithoutDatabaseNameIsError(t *testing.T) {
	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: "mongodb://127.0.0.1:27117"},
	})
	t.Cleanup(reg.Close)
	_, err := reg.RowCountMongo(context.Background(), "main", "users", nil)
	if err == nil {
		t.Fatal("expected an error for a MongoDB DSN with no database name in its path")
	}
}

func TestToBSONFilter_RejectsNonMappingFilter(t *testing.T) {
	if _, err := toBSONFilter("not-a-map"); err == nil {
		t.Fatal("expected a non-mapping filter to be rejected")
	}
}

func TestToBSONFilter_NilFilterBecomesEmptyMatch(t *testing.T) {
	f, err := toBSONFilter(nil)
	if err != nil {
		t.Fatalf("toBSONFilter(nil): %v", err)
	}
	if len(f) != 0 {
		t.Errorf("expected an empty filter, got %v", f)
	}
}

func TestMongoDatabaseNameFromDSN(t *testing.T) {
	cases := []struct {
		dsn     string
		want    string
		wantErr bool
	}{
		{"mongodb://localhost:27017/mydb", "mydb", false},
		{"mongodb://user:pass@localhost:27017/mydb?authSource=admin", "mydb", false},
		{"mongodb+srv://user:pass@cluster0.mongodb.net/mydb", "mydb", false},
		{"mongodb://localhost:27017", "", true},
		{"mongodb://localhost:27017/", "", true},
	}
	for _, c := range cases {
		got, err := mongoDatabaseNameFromDSN(c.dsn)
		if c.wantErr {
			if err == nil {
				t.Errorf("dsn %q: expected an error", c.dsn)
			}
			continue
		}
		if err != nil {
			t.Errorf("dsn %q: unexpected error: %v", c.dsn, err)
			continue
		}
		if got != c.want {
			t.Errorf("dsn %q: got %q, want %q", c.dsn, got, c.want)
		}
	}
}

func TestListCollectionNames_ReturnsRealCollections(t *testing.T) {
	skipIfNoMongo(t)

	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)

	// Seed two distinctly-named collections so there is something
	// real to find.
	db, err := reg.getMongo("main")
	if err != nil {
		t.Fatalf("getMongo: %v", err)
	}
	ctx := context.Background()
	if _, err := db.Collection("apilens_list_test_a").InsertOne(ctx, map[string]any{"x": 1}); err != nil {
		t.Fatalf("seeding collection a: %v", err)
	}
	if _, err := db.Collection("apilens_list_test_b").InsertOne(ctx, map[string]any{"x": 1}); err != nil {
		t.Fatalf("seeding collection b: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Collection("apilens_list_test_a").Drop(ctx)
		_ = db.Collection("apilens_list_test_b").Drop(ctx)
	})

	names, err := reg.ListCollectionNames("main")
	if err != nil {
		t.Fatalf("ListCollectionNames: %v", err)
	}

	found := map[string]bool{}
	for _, n := range names {
		found[n] = true
	}
	if !found["apilens_list_test_a"] {
		t.Errorf("expected apilens_list_test_a in %v", names)
	}
	if !found["apilens_list_test_b"] {
		t.Errorf("expected apilens_list_test_b in %v", names)
	}
}

func TestListCollectionNames_UnconfiguredConnectionIsError(t *testing.T) {
	reg := NewRegistry(nil)
	_, err := reg.ListCollectionNames("does-not-exist")
	if err == nil {
		t.Fatal("expected an error for an unconfigured connection name")
	}
}

func TestListCollectionNames_SQLConnectionIsError(t *testing.T) {
	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "sqlite", DSN: ":memory:"},
	})
	t.Cleanup(reg.Close)
	_, err := reg.ListCollectionNames("main")
	if err == nil {
		t.Fatal("expected an error when a sqlite-configured connection is asked for MongoDB collection names")
	}
}

func TestRowCountMongo_HexStringFilterMatchesRealObjectIDField(t *testing.T) {
	// Regression test for a real bug found while building this feature:
	// a document's _id is commonly stored as a genuine BSON ObjectID, not
	// a string. A naive filter of {"_id": "<hex string>"} would silently
	// match zero documents against such a field unless toBSONFilter
	// coerces hex-looking string values to also match the ObjectID form.
	skipIfNoMongo(t)

	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)

	db, err := reg.getMongo("main")
	if err != nil {
		t.Fatalf("getMongo: %v", err)
	}
	ctx := context.Background()
	coll := db.Collection("apilens_objectid_coercion_test")
	t.Cleanup(func() { _ = coll.Drop(ctx) })

	oid := bson.NewObjectID()
	if _, err := coll.InsertOne(ctx, bson.M{"_id": oid, "name": "ada"}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	// The exact shape internal/generate's __typename walker would
	// produce: a plain hex string copied straight from a JSON response.
	count, err := reg.RowCountMongo(ctx, "main", "apilens_objectid_coercion_test", map[string]any{"_id": oid.Hex()})
	if err != nil {
		t.Fatalf("RowCountMongo: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (hex string filter should match a real ObjectID _id field)", count)
	}
}

func TestRowCountMongo_HexStringFilterStillMatchesStringIDField(t *testing.T) {
	// The coercion must not break the (also common) case where _id is
	// genuinely stored as a plain string, not an ObjectID.
	skipIfNoMongo(t)

	reg := NewRegistry(map[string]ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)

	db, err := reg.getMongo("main")
	if err != nil {
		t.Fatalf("getMongo: %v", err)
	}
	ctx := context.Background()
	coll := db.Collection("apilens_string_id_test")
	t.Cleanup(func() { _ = coll.Drop(ctx) })

	stringID := "6a81dcfa6f6ebb3f76b802d4" // hex-looking, but stored as a plain string _id
	if _, err := coll.InsertOne(ctx, bson.M{"_id": stringID, "name": "ada"}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	count, err := reg.RowCountMongo(ctx, "main", "apilens_string_id_test", map[string]any{"_id": stringID})
	if err != nil {
		t.Fatalf("RowCountMongo: %v", err)
	}
	if count != 1 {
		t.Errorf("count = %d, want 1 (hex-looking filter should still match a genuine string _id)", count)
	}
}

func TestCoerceObjectIDLookingValues_LeavesNonHexStringsUnchanged(t *testing.T) {
	m := map[string]any{"email": "ada@example.com", "status": "paid"}
	out := coerceObjectIDLookingValues(m)
	if out["email"] != "ada@example.com" {
		t.Errorf("email = %v, want unchanged", out["email"])
	}
	if out["status"] != "paid" {
		t.Errorf("status = %v, want unchanged", out["status"])
	}
}

func TestCoerceObjectIDLookingValues_LeavesNestedOperatorsUnchanged(t *testing.T) {
	m := map[string]any{"total": map[string]any{"$gt": 100}}
	out := coerceObjectIDLookingValues(m)
	nested, ok := out["total"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested operator map to survive unchanged, got %#v", out["total"])
	}
	if nested["$gt"] != 100 {
		t.Errorf("$gt = %v, want 100", nested["$gt"])
	}
}
