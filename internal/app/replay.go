package app

import (
	"context"

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

// Generate looks up displayID in history and writes a YAML v1 test from
// it (docs/08-proxy.md section 9).
func (a *App) Generate(displayID int, opts generate.Options) (generate.Generated, error) {
	stored, ok, err := a.HistoryGet(displayID)
	if err != nil {
		return generate.Generated{}, err
	}
	if !ok {
		return generate.Generated{}, domain.NewNotFoundError(
			"no history entry — run apilens watch first, or check the id")
	}

	svc := generate.New(a.ProjectDir)
	return svc.FromExchange(stored, opts)
}
