package dbassert

// Importing these packages for their side effect of registering a
// database/sql driver (blank import). sqlite (modernc.org/sqlite) is
// pure Go — no CGO, so it builds the same way on every platform ApiLens
// already targets (docs/05-cli.md's CI-image requirement extends
// naturally to "no C toolchain needed"). postgres uses jackc/pgx's own
// database/sql-compatible driver (registers as "pgx").
import (
	_ "github.com/jackc/pgx/v5/stdlib"
	_ "modernc.org/sqlite"
)
