package webapi

import (
	"net/http"

	"github.com/sandeepv/apilens/pkg/apilens"
)

// handleListEndpoints backs the API Explorer tab (plan.md v6).
// GET /api/endpoints?method=&path=&tag=&source=
func (s *Server) handleListEndpoints(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	endpoints := s.engine.List(apilens.EndpointFilter{
		Method: q.Get("method"),
		Path:   q.Get("path"),
		Tag:    q.Get("tag"),
		Source: q.Get("source"),
	})
	if endpoints == nil {
		endpoints = []apilens.Endpoint{}
	}
	writeJSON(w, http.StatusOK, endpoints)
}

// discoverRequest is the POST /api/discover body.
type discoverRequest struct {
	Source []string `json:"source"`
	Path   []string `json:"path"`
}

// handleDiscover triggers discovery (plan.md v6 Explorer's "Discover"
// action). POST /api/discover
func (s *Server) handleDiscover(w http.ResponseWriter, r *http.Request) {
	var req discoverRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.engine.Discover(r.Context(), apilens.DiscoverOptions{Enabled: req.Source, Paths: req.Path})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// handleInspect backs the Explorer's detail view.
// GET /api/inspect?ref=&method=&live=true
func (s *Server) handleInspect(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if q.Get("ref") == "" {
		writeError(w, missingParam("ref"))
		return
	}
	insp, err := s.engine.Inspect(r.Context(), apilens.InspectRef{
		Ref:    q.Get("ref"),
		Method: q.Get("method"),
		Live:   q.Get("live") == "true",
	})
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, insp)
}
