package assertions

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/dbassert"
	"github.com/sandeepv/apilens/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func mongoTestDSN() string {
	if v := os.Getenv("APILENS_TEST_MONGO_DSN"); v != "" {
		return v
	}
	return "mongodb://127.0.0.1:27017/apilens_dbassert_test"
}

// skipIfNoMongo mirrors dbassert's own test helper of the same purpose —
// duplicated rather than exported since it is only ever needed by tests.
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

func newMongoRegistryWithSeededOrders(t *testing.T) *dbassert.Registry {
	t.Helper()
	skipIfNoMongo(t)

	reg := dbassert.NewRegistry(map[string]dbassert.ConnectionConfig{
		"main": {Driver: "mongodb", DSN: mongoTestDSN()},
	})
	t.Cleanup(reg.Close)

	// Seed through a plain driver connection (dbassert.Registry has no
	// exported "raw client" accessor, same discipline as the SQL tests
	// in dbcheck_test.go).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(mongoTestDSN()))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	t.Cleanup(func() { _ = client.Disconnect(context.Background()) })
	coll := client.Database("apilens_dbassert_test").Collection("orders")
	if _, err := coll.DeleteMany(ctx, bson.M{}); err != nil {
		t.Fatalf("clearing collection: %v", err)
	}
	if _, err := coll.InsertOne(ctx, bson.M{"_id": "o1", "status": "paid", "total": 42}); err != nil {
		t.Fatalf("seeding document: %v", err)
	}
	return reg
}

func TestEngine_DBMongoExists_TrueAndFalse(t *testing.T) {
	reg := newMongoRegistryWithSeededOrders(t)
	e := New(WithDBRegistry(reg))
	yes := true
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"status": "paid"}, Exists: &yes}},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if !results[0].Passed {
		t.Errorf("expected db.main exists=true to pass for a matching filter, got %+v", results[0])
	}

	set2, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"status": "refunded"}, Exists: &yes}},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results2 := e.Eval(set2, exchangeIgnoringBody())
	if results2[0].Passed {
		t.Error("expected exists=true to fail for a non-matching filter")
	}
}

func TestEngine_DBMongoRowCountEquals(t *testing.T) {
	reg := newMongoRegistryWithSeededOrders(t)
	e := New(WithDBRegistry(reg))
	want := 1
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"status": "paid"}, RowCountEquals: &want}},
		},
	})
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	results := e.Eval(set, exchangeIgnoringBody())
	if !results[0].Passed {
		t.Errorf("expected row_count_equals to pass, got %+v", results[0])
	}
}

func TestEngine_DBMongoEquals_ComparesNamedField(t *testing.T) {
	reg := newMongoRegistryWithSeededOrders(t)
	e := New(WithDBRegistry(reg))
	set, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"_id": "o1"}, Field: "status", Equals: "paid"}},
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

func TestEngine_DBMongoAssertion_DoesNotRequireReadOnlyKeywordScan(t *testing.T) {
	// A MongoDB block has no SQL string, so it must never be rejected by
	// dbassert.ValidateReadOnly's keyword scan (which only applies to
	// Query) -- this compiles successfully even though it never sets
	// Query at all.
	reg := newMongoRegistryWithSeededOrders(t)
	e := New(WithDBRegistry(reg))
	yes := true
	_, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"status": "paid"}, Exists: &yes}},
		},
	})
	if err != nil {
		t.Fatalf("expected a MongoDB db.* block to compile without a SQL query, got %v", err)
	}
}

func TestEngine_DBMongoAssertion_WithoutRegistryIsConfigError(t *testing.T) {
	e := New() // no WithDBRegistry
	yes := true
	_, err := e.Compile(domain.AssertionSpec{
		DB: map[string][]domain.DBSpec{
			"main": {{Collection: "orders", Filter: map[string]any{"status": "paid"}, Exists: &yes}},
		},
	})
	if err == nil {
		t.Fatal("expected a config error when db.* (mongo shape) is used with no registry configured")
	}
}
