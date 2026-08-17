package app

import (
	"context"
	"fmt"

	"github.com/sandeepv/apilens/internal/dbassert"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
	"github.com/sandeepv/apilens/internal/replay"
	"github.com/sandeepv/apilens/internal/runner"
)

// Replay looks up displayID in history (in-memory, falling back to the
// session file — same resolution as HistoryGet), applies ov, and executes
// the reconstructed request through the shared runner. The result is
// appended to history as a NEW entry — it never overwrites the one it
// replayed (docs/08-proxy.md section 8).
func (a *App) Replay(ctx context.Context, displayID int, ov replay.Overrides) (domain.Exchange, error) {
	stored, ok, err := a.HistoryGet(displayID)
	if err != nil {
		return domain.Exchange{}, err
	}
	if !ok {
		return domain.Exchange{}, domain.NewNotFoundError(
			"no history entry — run apilens watch first, or check the id")
	}

	httpRunner := runner.New(runner.WithMaxResponseSize(a.Config.MaxResponseSizeBytes()))
	svc := replay.New(httpRunner)

	result, err := svc.Replay(ctx, stored, ov)
	if err != nil {
		return domain.Exchange{}, err
	}

	return a.History.Append(result), nil
}

// GenerateOptions wraps generate.Options with the one extra choice that
// belongs at the App layer, not generate's: whether to derive db.*
// assertions from the response's "__typename"/"_id" fields
// (docs/07-discovery.md's "never invent" principle applied to Mongo
// collection names — see internal/generate/dbhints.go). generate itself
// has no idea what a "db connection" is; only App can build the
// dbassert.Registry a CollectionLister needs, the same way
// buildAssertEngine already does for RunSuite.
type GenerateOptions struct {
	generate.Options
	// DBHints enables __typename-based db.* auto-assertion. Ignored
	// (silently) if the project has no db.connections configured at
	// all — there is nothing to verify a guess against.
	DBHints bool
	// DBConnection names which configured connection to check guessed
	// collections against. Empty defaults to "main" if configured, or
	// the project's only configured connection if there is exactly one.
	DBConnection string
}

// Generate looks up displayID in history and writes a YAML v1 test from
// it (docs/08-proxy.md section 9).
func (a *App) Generate(displayID int, opts GenerateOptions) (generate.Generated, error) {
	stored, ok, err := a.HistoryGet(displayID)
	if err != nil {
		return generate.Generated{}, err
	}
	if !ok {
		return generate.Generated{}, domain.NewNotFoundError(
			"no history entry — run apilens watch first, or check the id")
	}

	genOpts := opts.Options
	if opts.DBHints {
		connName, reg, err := a.dbHintRegistry(opts.DBConnection)
		if err != nil {
			return generate.Generated{}, err
		}
		if reg != nil {
			defer reg.Close()
			genOpts.DBHints = &generate.DBHintOptions{Connection: connName, Lister: reg}
		}
	}

	svc := generate.New(a.ProjectDir)
	return svc.FromExchange(stored, genOpts)
}

// dbHintRegistry resolves which configured db.connections entry
// --db-hints should verify guessed collection names against, and opens a
// dbassert.Registry for it. Returns (name, nil, nil) — not an error — when
// no connection is configured at all, so `apilens generate --db-hints` on
// a project with no db.connections silently does nothing rather than
// failing a capture that has nothing to do with a database.
func (a *App) dbHintRegistry(requested string) (string, *dbassert.Registry, error) {
	if len(a.Config.DB.Connections) == 0 {
		return "", nil, nil
	}

	connName := requested
	if connName == "" {
		if _, ok := a.Config.DB.Connections["main"]; ok {
			connName = "main"
		} else if len(a.Config.DB.Connections) == 1 {
			for name := range a.Config.DB.Connections {
				connName = name
			}
		} else {
			return "", nil, domain.NewConfigError(
				fmt.Sprintf("--db-hints needs --db-connection: multiple connections are configured (%d) and none is named \"main\"", len(a.Config.DB.Connections)), nil)
		}
	}
	if _, ok := a.Config.DB.Connections[connName]; !ok {
		return "", nil, domain.NewConfigError(
			fmt.Sprintf("--db-connection %q is not configured — add db.connections.%s to config.yaml", connName, connName), nil)
	}

	conns := make(map[string]dbassert.ConnectionConfig, len(a.Config.DB.Connections))
	for name, c := range a.Config.DB.Connections {
		conns[name] = dbassert.ConnectionConfig{Driver: c.Driver, DSN: c.DSN}
	}
	return connName, dbassert.NewRegistry(conns), nil
}
