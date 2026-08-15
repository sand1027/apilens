// Package webapi implements the JSON REST + SSE surface for the local web
// dashboard (plan.md v6). It is an adapter exactly like internal/cli: every
// handler translates HTTP into a pkg/apilens.Engine call and back
// (docs/01-architecture.md section 7, "shared engine rule" — "If a
// feature exists in the UI, it must already exist as an engine method").
// No business logic lives here.
package webapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

// Server wraps an Engine with an http.Handler. Construct with New and
// mount at "/" (it serves both /api/* and the embedded frontend).
type Server struct {
	engine    apilens.Engine
	mux       *http.ServeMux
	watchCtrl *watchController
}

// New builds a Server. frontend, if non-nil, is served for any path that
// doesn't match "/api/" — the embedded Next.js static export
// (docs/02-packages.md / plan.md v6: "web/dashboard/ ... Go serves it").
func New(engine apilens.Engine, frontend http.Handler) *Server {
	s := &Server{
		engine:    engine,
		mux:       http.NewServeMux(),
		watchCtrl: newWatchController(engine),
	}
	s.routes(frontend)
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	applyLoopbackCORS(w, r)
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes(frontend http.Handler) {
	s.mux.HandleFunc("GET /api/endpoints", s.handleListEndpoints)
	s.mux.HandleFunc("POST /api/discover", s.handleDiscover)
	s.mux.HandleFunc("GET /api/inspect", s.handleInspect)
	s.mux.HandleFunc("POST /api/run", s.handleRun)
	s.mux.HandleFunc("GET /api/history", s.handleHistoryList)
	s.mux.HandleFunc("GET /api/history/{id}", s.handleHistoryGet)
	s.mux.HandleFunc("POST /api/replay/{id}", s.handleReplay)
	s.mux.HandleFunc("POST /api/generate/{id}", s.handleGenerate)
	s.mux.HandleFunc("GET /api/environments", s.handleListEnvironments)
	s.mux.HandleFunc("POST /api/environments/use", s.handleUseEnvironment)
	s.mux.HandleFunc("POST /api/watch/start", s.watchCtrl.handleStart)
	s.mux.HandleFunc("POST /api/watch/stop", s.watchCtrl.handleStop)
	s.mux.HandleFunc("GET /api/watch/status", s.watchCtrl.handleStatus)
	s.mux.HandleFunc("GET /api/watch/events", s.watchCtrl.handleEvents)
	s.mux.HandleFunc("GET /widget.js", handleWidgetJS)

	if frontend != nil {
		s.mux.Handle("/", frontend)
	}
}

// writeJSON encodes v as the response body. Errors writing the response
// itself are not the caller's problem to handle (the connection is
// already compromised if this fails).
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// apiError is the JSON error shape every handler returns on failure.
type apiError struct {
	Error string `json:"error"`
}

// writeError maps an Engine error to an HTTP status using the same
// classification internal/cli/exit.go uses for process exit codes
// (ErrConfig/ErrNotFound/ErrSecurity -> 4xx, ErrTransport/unknown -> 502/500)
// so the dashboard and the CLI never disagree about what a given error
// means (docs/04-interfaces.md section 15).
func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
	case errors.Is(err, domain.ErrConfig):
		status = http.StatusBadRequest
	case errors.Is(err, domain.ErrSecurity):
		status = http.StatusForbidden
	case errors.Is(err, domain.ErrNotImplemented):
		status = http.StatusNotImplemented
	case errors.Is(err, domain.ErrTransport):
		status = http.StatusBadGateway
	}
	writeJSON(w, status, apiError{Error: err.Error()})
}

// decodeJSON reads and decodes a JSON request body, returning a
// domain.ErrConfig-classified error on failure so writeError maps it to
// 400 rather than 500.
func decodeJSON(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		return domain.NewConfigError("decoding request body", err)
	}
	return nil
}

// missingParam builds a consistent ErrConfig for a required query/path
// parameter that the caller didn't supply.
func missingParam(name string) error {
	return domain.NewConfigError("missing required parameter \""+name+"\"", nil)
}
