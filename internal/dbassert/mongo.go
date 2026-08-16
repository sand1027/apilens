package dbassert

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// mongoConnectTimeout bounds how long opening a new MongoDB connection
// may take (server selection + handshake), mirroring dbQueryTimeout's
// "no unbounded external call" discipline in internal/assertions —
// applied here at connect time rather than query time, since
// mongo.Connect itself does not verify reachability (per its own doc
// comment) and a bad host should fail fast rather than hang until the
// query's own timeout.
const mongoConnectTimeout = 10 * time.Second

// getMongo returns (creating and caching, if necessary) the *mongo.Database
// for connName. Mirrors get's shape for SQL connections, but MongoDB
// needs its own client type (mongo.Client, not *sql.DB) since it is not
// a database/sql driver.
func (r *Registry) getMongo(name string) (*mongo.Database, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if db, ok := r.mongoConns[name]; ok {
		return db, nil
	}
	cfg, ok := r.dsns[name]
	if !ok {
		return nil, fmt.Errorf("no db connection named %q is configured (db.<connections>.%s in config.yaml)", name, name)
	}
	if !isMongoDriver(cfg.Driver) {
		return nil, fmt.Errorf("db connection %q is configured with driver %q, not mongodb — use \"query\" in db.%s, not \"collection\"", name, cfg.Driver, name)
	}
	dsn, err := expandDSN(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("db connection %q: %w", name, err)
	}
	dbName, err := mongoDatabaseNameFromDSN(dsn)
	if err != nil {
		return nil, fmt.Errorf("db connection %q: %w", name, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), mongoConnectTimeout)
	defer cancel()
	opts := options.Client().ApplyURI(dsn).SetServerSelectionTimeout(mongoConnectTimeout)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, fmt.Errorf("opening db connection %q: %w", name, err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(context.Background())
		return nil, fmt.Errorf("connecting to db connection %q: %w", name, err)
	}

	db := client.Database(dbName)
	r.mongoClients[name] = client
	r.mongoConns[name] = db
	return db, nil
}

// mongoDatabaseNameFromDSN extracts the database name from a
// "mongodb://" or "mongodb+srv://" connection string's path segment
// (e.g. "mongodb://localhost:27017/mydb" -> "mydb") — mongo.Connect
// itself never selects a database; callers must always name one via
// Client.Database(name), so the DSN's path is the one place a project's
// config.yaml can name it without a second config field.
func mongoDatabaseNameFromDSN(dsn string) (string, error) {
	u, err := url.Parse(dsn)
	if err != nil {
		return "", fmt.Errorf("invalid MongoDB connection string: %w", err)
	}
	name := strings.TrimPrefix(u.Path, "/")
	if name == "" {
		return "", fmt.Errorf("MongoDB connection string must include a database name, e.g. \"mongodb://host:27017/mydb\"")
	}
	return name, nil
}

// RowCountMongo runs filter against collection and returns the number of
// matching documents — the MongoDB analogue of RowCount.
func (r *Registry) RowCountMongo(ctx context.Context, connName, collection string, filter any) (int, error) {
	db, err := r.getMongo(connName)
	if err != nil {
		return 0, err
	}
	bsonFilter, err := toBSONFilter(filter)
	if err != nil {
		return 0, fmt.Errorf("db.%s.filter: %w", connName, err)
	}
	count, err := db.Collection(collection).CountDocuments(ctx, bsonFilter)
	if err != nil {
		return 0, fmt.Errorf("running db query on %q.%s: %w", connName, collection, err)
	}
	return int(count), nil
}

// FirstValueMongo runs filter against collection, returning field of the
// first matched document formatted as a string — the MongoDB analogue of
// FirstValue. field defaults to "_id" when empty (the one field every
// document is guaranteed to have).
func (r *Registry) FirstValueMongo(ctx context.Context, connName, collection string, filter any, field string) (string, bool, error) {
	db, err := r.getMongo(connName)
	if err != nil {
		return "", false, err
	}
	if field == "" {
		field = "_id"
	}
	bsonFilter, err := toBSONFilter(filter)
	if err != nil {
		return "", false, fmt.Errorf("db.%s.filter: %w", connName, err)
	}
	raw, err := db.Collection(collection).FindOne(ctx, bsonFilter).Raw()
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return "", false, nil
		}
		return "", false, fmt.Errorf("running db query on %q.%s: %w", connName, collection, err)
	}
	val := raw.Lookup(field)
	if val.IsZero() {
		return "", false, fmt.Errorf("field %q not present on matched document in %q.%s", field, connName, collection)
	}
	return formatBSONValue(val), true, nil
}

// toBSONFilter converts a filter decoded from YAML (a plain
// map[string]any / []any / scalar tree, per gopkg.in/yaml.v3's default
// decoding — see internal/testdef.document's use of `any` fields) into a
// bson.M the driver can send as-is. YAML's map[string]any keys are
// already strings, so this is a type assertion, not a real conversion —
// but it fails closed with a clear error rather than passing a
// non-object filter (e.g. a bare string) straight to the driver, which
// would otherwise surface as an opaque marshal error deep in the mongo
// package.
func toBSONFilter(filter any) (bson.M, error) {
	if filter == nil {
		return bson.M{}, nil
	}
	m, ok := filter.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("must be a mapping (e.g. {email: \"a@b.com\"}), got %T", filter)
	}
	return bson.M(m), nil
}

// formatBSONValue renders a BSON scalar as a plain string for db.equals
// comparisons, mirroring dbassert.formatScanValue's role for SQL scalar
// columns. ObjectID renders as its hex string (the form every MongoDB
// user recognizes and would write in YAML), not RawValue's default
// Extended JSON encoding.
func formatBSONValue(v bson.RawValue) string {
	switch v.Type {
	case bson.TypeObjectID:
		return v.ObjectID().Hex()
	case bson.TypeString:
		return v.StringValue()
	case bson.TypeInt32:
		return strconv.FormatInt(int64(v.Int32()), 10)
	case bson.TypeInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case bson.TypeDouble:
		return strconv.FormatFloat(v.Double(), 'f', -1, 64)
	case bson.TypeBoolean:
		return strconv.FormatBool(v.Boolean())
	case bson.TypeNull:
		return ""
	default:
		return v.String()
	}
}
