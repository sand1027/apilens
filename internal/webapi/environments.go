package webapi

import "net/http"

// handleListEnvironments backs the Environment switcher (plan.md v6).
// GET /api/environments
func (s *Server) handleListEnvironments(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, struct {
		Current string `json:"current"`
		Items   any    `json:"items"`
	}{
		Current: string(s.engine.CurrentEnv().Name),
		Items:   s.engine.Environments(),
	})
}

// useEnvironmentRequest is the POST /api/environments/use body.
type useEnvironmentRequest struct {
	Name string `json:"name"`
}

// handleUseEnvironment switches the active environment. Unlike `apilens
// env use`, this does NOT persist to .apilens/.current-env — a dashboard
// session's environment choice is a UI-session concern (the Engine
// instance backing the running `apilens ui` process), not a project-wide
// default change a browser tab should silently make on disk.
// POST /api/environments/use
func (s *Server) handleUseEnvironment(w http.ResponseWriter, r *http.Request) {
	var req useEnvironmentRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	if req.Name == "" {
		writeError(w, missingParam("name"))
		return
	}
	if err := s.engine.UseEnv(req.Name); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.engine.CurrentEnv())
}
