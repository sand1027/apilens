package webapi

import (
	"net/http"

	"github.com/sandeepv/apilens/pkg/apilens"
)

// runRequest is the POST /api/run body, mirroring apilens.RunFilter
// (docs/05-cli.md `apilens run` flags) — every field the CLI exposes, the
// dashboard's Test Runner tab exposes too (plan.md v6: "Every UI action is
// an Engine method that the CLI can already perform").
type runRequest struct {
	Ref        string `json:"ref"`
	Method     string `json:"method"`
	Tag        string `json:"tag"`
	Sequential bool   `json:"sequential"`
	Parallel   bool   `json:"parallel"`
	FailFast   bool   `json:"failFast"`
}

// handleRun executes the suite (or a filtered subset) and returns the
// domain.Report as JSON — the same shape the CLI's --format json produces
// (docs/11-risks-and-gaps.md G25), so the dashboard's result view and the
// CLI's JSON reporter agree on structure.
// POST /api/run
func (s *Server) handleRun(w http.ResponseWriter, r *http.Request) {
	var req runRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	report, err := s.engine.Run(r.Context(), apilens.RunFilter{
		Ref:            req.Ref,
		Method:         req.Method,
		Tag:            req.Tag,
		Sequential:     req.Sequential,
		Parallel:       req.Parallel,
		FailFast:       req.FailFast,
		ReporterFormat: "json",
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
