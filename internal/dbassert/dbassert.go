// Package dbassert implements plan.md v9's "Database assertions (opt-in
// plugin)": after an HTTP call, run a read-only SQL query and assert on
// its result — e.g. confirm a POST actually persisted a row, or that a
// DELETE removed one. This is opt-in for two independent reasons:
//
//  1. It requires a database connection string, which is a credential.
//     ApiLens never connects anywhere by default (docs/09-security.md's
//     "no network surprises" posture) — a project must explicitly
//     configure db.<connection>.dsn before any db.* assertion can even
//     compile.
//  2. It adds a real external dependency (a SQL driver) to the request
//     path. Projects that never use it should pay zero connection-pool
//     or query-latency cost.
//
// Only read (SELECT) queries are permitted — dbassert refuses to compile
// a query containing an INSERT/UPDATE/DELETE/DROP/ALTER/CREATE/TRUNCATE
// keyword (case-insensitive, word-boundary matched) so a copy-pasted
// mutating query can never accidentally run as a "test assertion" side
// effect against a real database (docs/11-risks-and-gaps.md's "never
// invent/never mutate what you were only asked to observe" discipline,
// extended to SQL).
package dbassert

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/sandeepv/apilens/internal/domain"
)

// Registry holds open, opt-in-configured database connections, keyed by
// the connection name used in the YAML DSL (db.<connection>.query...).
// Connections are opened lazily on first use and cached for the life of
// the process — the same posture internal/runner's shared http.Client
// takes toward keeping one pool alive across a whole suite run rather
// than reconnecting per test.
type Registry struct {
	mu    sync.Mutex
	conns map[string]*sql.DB
	dsns  map[string]ConnectionConfig
}

// ConnectionConfig names one configured connection (plan.md v9: opt-in —
// there is no default connection).
type ConnectionConfig struct {
	Driver string // "sqlite" or "postgres" (v9 ships both; more drivers can register later)
	DSN    string
}

// NewRegistry builds a Registry over the given named connections. An
// empty/nil conns map is valid — it simply means no db.* assertion can
// compile (every reference to an unconfigured connection name is a
// config error, per docs/06-test-dsl.md section 12's compile-time
// validation discipline).
func NewRegistry(conns map[string]ConnectionConfig) *Registry {
	return &Registry{
		conns: make(map[string]*sql.DB),
		dsns:  conns,
	}
}

// Close closes every opened connection. Called once at process/suite
// shutdown, mirroring how internal/runner's http.Client is shared for a
// whole run rather than torn down per request.
func (r *Registry) Close() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, db := range r.conns {
		_ = db.Close()
	}
}

func (r *Registry) get(name string) (*sql.DB, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if db, ok := r.conns[name]; ok {
		return db, nil
	}
	cfg, ok := r.dsns[name]
	if !ok {
		return nil, fmt.Errorf("no db connection named %q is configured (db.<connections>.%s in config.yaml)", name, name)
	}
	driverName, err := registeredDriverName(cfg.Driver)
	if err != nil {
		return nil, err
	}
	dsn, err := expandDSN(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("db connection %q: %w", name, err)
	}
	db, err := sql.Open(driverName, dsn)
	if err != nil {
		return nil, fmt.Errorf("opening db connection %q: %w", name, err)
	}
	r.conns[name] = db
	return db, nil
}

// dsnEnvPattern mirrors environment.envPattern's "${NAME}" syntax — a DSN
// is a credential, so config.yaml should reference it via an environment
// variable rather than embedding a plaintext password (docs/09-security.md's
// "never in git" posture, applied here the same way it already applies to
// auth tokens in environment files).
var dsnEnvPattern = regexp.MustCompile(`\$\{([a-zA-Z0-9_]+)\}`)

// expandDSN expands every "${NAME}" in dsn from the process environment,
// failing closed (ADR-015) if any referenced variable is unset — a DSN
// with a silently-empty password is worse than a clear startup error.
func expandDSN(dsn string) (string, error) {
	var missing []string
	result := dsnEnvPattern.ReplaceAllStringFunc(dsn, func(match string) string {
		name := dsnEnvPattern.FindStringSubmatch(match)[1]
		if v, ok := os.LookupEnv(name); ok {
			return v
		}
		missing = append(missing, name)
		return match
	})
	if len(missing) > 0 {
		return "", fmt.Errorf("missing required environment variable(s) for dsn: %s", strings.Join(missing, ", "))
	}
	return result, nil
}

// mutatingKeywordPattern matches any SQL statement keyword that writes,
// as a whole word (so a column literally named "updated_at" doesn't
// false-positive). Checked case-insensitively.
var mutatingKeywordPattern = regexp.MustCompile(
	`(?i)\b(insert|update|delete|drop|alter|create|truncate|grant|revoke|replace|merge)\b`)

// ValidateReadOnly rejects a query containing any mutating SQL keyword.
// Called at Compile time (before any connection is even opened), same
// discipline as assertions.Engine's schema/regex compile-time checks —
// bad DSL is caught before touching anything live.
func ValidateReadOnly(query string) error {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return fmt.Errorf("db query is empty")
	}
	if mutatingKeywordPattern.MatchString(trimmed) {
		return fmt.Errorf("db assertions may only run read (SELECT) queries — %q contains a mutating keyword", query)
	}
	if !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(trimmed)), "SELECT") {
		return fmt.Errorf("db assertions must start with SELECT, got %q", query)
	}
	return nil
}

// RowCount runs query (already validated read-only) and returns the
// number of rows returned — the query itself decides what "count" means
// (e.g. "SELECT id FROM users WHERE email = ?" for existence, or
// "SELECT COUNT(*) FROM ..." for a literal count comparison via
// FirstValue instead). ctx should carry the suite/test's own timeout so a
// slow or hung query can't stall the whole run indefinitely.
func (r *Registry) RowCount(ctx context.Context, connName, query string, args ...any) (int, error) {
	db, err := r.get(connName)
	if err != nil {
		return 0, err
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("running db query on %q: %w", connName, err)
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
	}
	if err := rows.Err(); err != nil {
		return 0, fmt.Errorf("reading db query results from %q: %w", connName, err)
	}
	return count, nil
}

// FirstValue runs query and returns the first column of the first row as
// a string (via database/sql's Scan-into-any-then-format, so it works
// uniformly across integer/text/bool/float column types) — used for
// db.<connection>.equals-style assertions comparing a single scalar.
func (r *Registry) FirstValue(ctx context.Context, connName, query string, args ...any) (string, bool, error) {
	db, err := r.get(connName)
	if err != nil {
		return "", false, err
	}
	row := db.QueryRowContext(ctx, query, args...)
	var val any
	if err := row.Scan(&val); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, fmt.Errorf("running db query on %q: %w", connName, err)
	}
	return formatScanValue(val), true, nil
}

func formatScanValue(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

// registeredDriverName maps a friendly driver name from config.yaml to
// the database/sql driver name actually registered by the imported
// driver package. Kept as an explicit allow-list (rather than passing the
// user's string straight to sql.Open) so a typo in config.yaml produces a
// clear "unsupported driver" config error instead of a generic
// "sql: unknown driver" panic-adjacent message deep inside database/sql.
func registeredDriverName(name string) (string, error) {
	switch strings.ToLower(name) {
	case "sqlite", "sqlite3":
		return "sqlite", nil
	case "postgres", "postgresql", "pgx":
		return "pgx", nil
	default:
		return "", fmt.Errorf("unsupported db driver %q (supported: sqlite, postgres)", name)
	}
}

// AssertionResult-building helpers mirror internal/assertions' own
// result-shape conventions, so db.* checks look and behave like every
// other assertion kind in reports (docs/04-interfaces.md section 7).

func passResult(kind domain.AssertionKind, target, expected, actual string) domain.AssertionResult {
	return domain.AssertionResult{Kind: kind, Target: target, Passed: true, Expected: expected, Actual: actual}
}

func failResult(kind domain.AssertionKind, target, expected, actual, reason string) domain.AssertionResult {
	return domain.AssertionResult{Kind: kind, Target: target, Passed: false, Expected: expected, Actual: actual, Reason: reason}
}
