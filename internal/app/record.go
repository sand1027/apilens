package app

import (
	"path/filepath"

	"github.com/sandeepv/apilens/internal/recording"
)

// RecordOptions configures Record (plan.md v9: "Test recording
// sessions").
type RecordOptions struct {
	// Limit restricts to the most recent N captured exchanges; 0 means
	// "use everything currently in history" (same convention as
	// HistoryList).
	Limit int
	// Out is the output directory; empty defaults to
	// .apilens/tests/recorded.
	Out   string
	Force bool
}

// RecordResult reports what Record wrote.
type RecordResult struct {
	Files []string
	Steps int
}

// Record turns the current history (in-memory if `watch` is running in
// this process, otherwise the session file — same source HistoryList
// already uses) into a chained DSL v2 suite on disk. It is the natural
// companion to `apilens watch` + `apilens generate`: where generate turns
// ONE captured call into ONE test, Record turns an entire recorded
// sequence into a suite that replays the sequence with real
// data-flow between steps (plan.md v9).
func (a *App) Record(opts RecordOptions) (RecordResult, error) {
	exchanges := a.HistoryList(opts.Limit)

	steps, err := recording.Session(exchanges)
	if err != nil {
		return RecordResult{}, err
	}

	out := opts.Out
	if out == "" {
		out = filepath.Join(a.ProjectDir, ".apilens", "tests", "recorded")
	}

	written, err := recording.WriteSuite(steps, recording.WriteOptions{Dir: out, Force: opts.Force})
	if err != nil {
		return RecordResult{Files: written}, err
	}
	return RecordResult{Files: written, Steps: len(steps)}, nil
}
