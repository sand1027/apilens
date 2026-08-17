package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func appTestMongoDSN(dbName string) string {
	if v := os.Getenv("APILENS_TEST_MONGO_DSN"); v != "" {
		return v
	}
	return "mongodb://127.0.0.1:27017/" + dbName
}

func skipIfNoMongoForApp(t *testing.T, dsn string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	client, err := mongo.Connect(options.Client().ApplyURI(dsn).SetServerSelectionTimeout(2 * time.Second))
	if err != nil {
		t.Skipf("no MongoDB available for integration test: %v", err)
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		t.Skipf("no MongoDB available for integration test: %v", err)
	}
}

// TestGenerate_DBHintsAgainstRealMongoDBConnection is the end-to-end
// verification for the whole __typename-hint feature, using the exact
// shape of the real createAdvance response captured earlier in this
// session: a top-level "Advance" object with "_id" and a nested "Receipt"
// with its own "_id". Configures a real db.connections entry pointed at
// a real MongoDB database (not a mock), seeds "advances" and "receipts"
// collections, and confirms apilens generate --db-hints actually writes
// a verified assert.db block referencing both.
func TestGenerate_DBHintsAgainstRealMongoDBConnection(t *testing.T) {
	dbName := "apilens_app_dbhints_test"
	dsn := appTestMongoDSN(dbName)
	skipIfNoMongoForApp(t, dsn)

	dir := t.TempDir()
	configDir := filepath.Join(dir, ".apilens")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	configYAML := "db:\n  connections:\n    main:\n      driver: mongodb\n      dsn: \"" + dsn + "\"\n"
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("writing config.yaml: %v", err)
	}

	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if len(a.Config.DB.Connections) != 1 {
		t.Fatalf("expected config.yaml's db.connections to load, got %+v", a.Config.DB.Connections)
	}

	// Seed real "advances" and "receipts" collections so the guessed
	// collection names have something real to verify against.
	client, err := mongo.Connect(options.Client().ApplyURI(dsn))
	if err != nil {
		t.Fatalf("mongo.Connect: %v", err)
	}
	defer client.Disconnect(context.Background())
	db := client.Database(dbName)
	ctx := context.Background()
	// Drop defensively before seeding too, in case a previous failed run
	// left the database dirty -- t.Cleanup below only protects the
	// common (test passes) case.
	_ = db.Collection("advances").Drop(ctx)
	_ = db.Collection("receipts").Drop(ctx)
	t.Cleanup(func() {
		_ = db.Collection("advances").Drop(ctx)
		_ = db.Collection("receipts").Drop(ctx)
	})
	advanceID := "6a81dcfa6f6ebb3f76b802d4"
	receiptID := "6a81dcfa6f6ebb3f76b802d9"
	if _, err := db.Collection("advances").InsertOne(ctx, bson.M{"_id": mustObjectIDFromHex(t, advanceID), "total": 20000}); err != nil {
		t.Fatalf("seeding advances: %v", err)
	}
	if _, err := db.Collection("receipts").InsertOne(ctx, bson.M{"_id": mustObjectIDFromHex(t, receiptID), "seqNo": "R-4858"}); err != nil {
		t.Fatalf("seeding receipts: %v", err)
	}

	// The exact shape captured from the real createAdvance mutation
	// earlier this session.
	responseBody := []byte(`{
		"data": {
			"createAdvance": {
				"__typename": "Advance",
				"_id": "` + advanceID + `",
				"total": 20000,
				"receipt": {
					"__typename": "Receipt",
					"_id": "` + receiptID + `",
					"seqNo": "R-4858"
				}
			}
		}
	}`)
	stored := a.History.Append(domain.Exchange{
		Request: domain.HTTPRequest{
			Method: "POST",
			URL:    "http://localhost:3000/graphql",
			Body:   []byte(`{"query":"mutation CreateAdvance($input: CreateAdvanceInput!) { createAdvance(input: $input) { _id } }","operationName":"CreateAdvance"}`),
		},
		Response: domain.HTTPResponse{StatusCode: 200, Body: responseBody},
	})

	g, err := a.Generate(int(stored.Display), GenerateOptions{DBHints: true})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(g.SkippedHints) != 0 {
		t.Errorf("expected no skipped hints (both collections are real), got %v", g.SkippedHints)
	}

	specs := g.Test.Assert.DB["main"]
	if len(specs) != 2 {
		t.Fatalf("expected 2 db checks (advances + receipts), got %+v", specs)
	}
	var sawAdvances, sawReceipts bool
	for _, s := range specs {
		if s.Collection == "advances" {
			sawAdvances = true
		}
		if s.Collection == "receipts" {
			sawReceipts = true
		}
	}
	if !sawAdvances || !sawReceipts {
		t.Errorf("expected checks against both advances and receipts, got %+v", specs)
	}

	// The written file must contain the block too, not just the
	// in-memory TestCase.
	content, err := os.ReadFile(g.Path)
	if err != nil {
		t.Fatalf("reading generated file: %v", err)
	}
	if !containsStr2(string(content), "collection: advances") || !containsStr2(string(content), "collection: receipts") {
		t.Errorf("generated YAML file missing expected db blocks:\n%s", content)
	}
}

func TestGenerate_DBHintsNoConnectionsConfiguredIsSilentNoOp(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir}) // no config.yaml at all -> no db.connections
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	stored := a.History.Append(domain.Exchange{
		Request:  domain.HTTPRequest{Method: "POST", URL: "http://localhost:3000/graphql"},
		Response: domain.HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`)},
	})

	g, err := a.Generate(int(stored.Display), GenerateOptions{DBHints: true})
	if err != nil {
		t.Fatalf("Generate: %v (should silently no-op, not error, when no db connections are configured)", err)
	}
	if len(g.Test.Assert.DB) != 0 {
		t.Errorf("expected no db checks when no connection is configured, got %+v", g.Test.Assert.DB)
	}
}

func mustObjectIDFromHex(t *testing.T, hex string) bson.ObjectID {
	t.Helper()
	oid, err := bson.ObjectIDFromHex(hex)
	if err != nil {
		t.Fatalf("ObjectIDFromHex(%q): %v", hex, err)
	}
	return oid
}

func containsStr2(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
