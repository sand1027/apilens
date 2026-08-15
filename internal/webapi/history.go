package webapi

import (
	"net/http"
	"strconv"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

// handleHistoryList backs the History tab. GET /api/history?limit=
func (s *Server) handleHistoryList(w http.ResponseWriter, r *http.Request) {
	limit := 0
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, domain.NewConfigError("invalid limit", err))
			return
		}
		limit = n
	}
	hist, err := s.engine.History(limit)
	if err != nil {
		writeError(w, err)
		return
	}
	if hist == nil {
		hist = []domain.Exchange{}
	}
	writeJSON(w, http.StatusOK, hist)
}

// handleHistoryGet backs the History tab's detail view.
// GET /api/history/{id}
func (s *Server) handleHistoryGet(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	ex, ok, err := s.engine.HistoryGet(id)
	if err != nil {
		writeError(w, err)
		return
	}
	if !ok {
		writeError(w, domain.NewNotFoundError("no history entry with that id"))
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

// replayRequest is the POST /api/replay/{id} body, mirroring
// apilens.ReplayOverrides / `apilens replay` flags.
type replayRequest struct {
	Method  *string           `json:"method"`
	URL     *string           `json:"url"`
	Headers map[string]string `json:"headers"`
	Unset   []string          `json:"unset"`
	Query   map[string]string `json:"query"`
}

// handleReplay backs the Request Builder tab. POST /api/replay/{id}
func (s *Server) handleReplay(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var req replayRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	ex, err := s.engine.Replay(r.Context(), id, apilens.ReplayOverrides{
		Method:  req.Method,
		URL:     req.URL,
		Headers: req.Headers,
		Unset:   req.Unset,
		Query:   req.Query,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ex)
}

// generateRequest is the POST /api/generate/{id} body.
type generateRequest struct {
	Out   string `json:"out"`
	Force bool   `json:"force"`
}

// handleGenerate writes a YAML test from a captured exchange.
// POST /api/generate/{id}
func (s *Server) handleGenerate(w http.ResponseWriter, r *http.Request) {
	id, err := parsePathID(r, "id")
	if err != nil {
		writeError(w, err)
		return
	}
	var req generateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.engine.Generate(id, apilens.GenerateOptions{Out: req.Out, Force: req.Force})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parsePathID(r *http.Request, name string) (int, error) {
	raw := r.PathValue(name)
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, domain.NewConfigError("invalid id \""+raw+"\"", err)
	}
	return n, nil
}
