// Package mock implements plan.md v8's "API mocking (apilens mock) using
// registry + captured examples". A mock server answers requests using
// whatever ApiLens already knows about an endpoint: the most recent
// captured exchange for it (preferred, since that's a real observed
// response), or — with no capture available — a minimal synthesized
// response built from the endpoint's declared status codes. It never
// contacts the real upstream (docs/08-proxy.md section 10's "Mocks ...
// are future product features" is now being delivered, following the
// same no-network-surprise discipline the proxy and runner use
// elsewhere).
package mock

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/security"
)

// Options configures Bind (docs/09-security.md section 5's loopback bind
// policy, applied identically to watch/ui).
type Options struct {
	Bind        string
	AllowRemote bool
}

// route is a matchable, pre-parsed version of a registry endpoint.
type route struct {
	method   domain.Method
	segments []string // canonical path split on "/", ":id" segments match anything
	endpoint domain.Endpoint
}

// Server serves mock responses for a fixed set of endpoints. Build with
// New, then Bind and Serve (same two-step lifecycle as internal/proxy.Proxy
// and for the same reason: callers that need to know synchronously
// whether the mock is actually listening call Bind before Serve).
type Server struct {
	routes   []route
	examples map[domain.EndpointID]domain.Exchange

	mu       sync.Mutex
	addr     string
	listener net.Listener
	server   *http.Server
}

// New builds a Server over endpoints, preferring examples[id] as the
// response for a matching request when present.
func New(endpoints []domain.Endpoint, examples map[domain.EndpointID]domain.Exchange) *Server {
	s := &Server{examples: examples}
	for _, ep := range endpoints {
		s.routes = append(s.routes, route{
			method:   ep.Method,
			segments: splitPath(domain.NormalizePath(ep.Path)),
			endpoint: ep,
		})
	}
	return s
}

// Addr returns the actual bound address once Bind has succeeded.
func (s *Server) Addr() string { return s.addr }

// Bind validates the bind policy (loopback only, unless AllowRemote) and
// opens the listener.
func (s *Server) Bind(opts Options) error {
	bind := opts.Bind
	if bind == "" {
		bind = "127.0.0.1:4489"
	}
	if err := security.ValidateListenAddr(bind, opts.AllowRemote); err != nil {
		return err
	}
	ln, err := net.Listen("tcp", bind)
	if err != nil {
		return domain.NewSecurityError("binding mock server: " + err.Error())
	}
	s.addr = ln.Addr().String()
	s.listener = ln
	return nil
}

// Serve runs the mock HTTP handler over the listener opened by Bind until
// ctx is canceled. Blocks — same contract as proxy.Proxy.Serve.
func (s *Server) Serve(ctx context.Context) error {
	if s.listener == nil {
		return domain.NewSecurityError("mock Serve called before Bind")
	}
	s.server = &http.Server{Handler: http.HandlerFunc(s.handle)}

	errCh := make(chan error, 1)
	go func() { errCh <- s.server.Serve(s.listener) }()

	select {
	case <-ctx.Done():
		_ = s.server.Close()
		return nil
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return domain.NewSecurityError("mock serve error: " + err.Error())
		}
		return nil
	}
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	matched := s.match(domain.NormalizeMethod(r.Method), r.URL.Path)
	if matched == nil {
		writeMockError(w, http.StatusNotFound, "no mock route registered for "+r.Method+" "+r.URL.Path)
		return
	}

	id := domain.NewEndpointID(matched.method, matched.endpoint.Path)
	if ex, ok := s.examples[id]; ok {
		writeCapturedExchange(w, ex)
		return
	}
	writeSyntheticResponse(w, matched.endpoint)
}

// match finds the first route whose method and path segments match,
// preferring an exact literal-segment match over a ":id"-containing one
// when both are candidates (e.g. "/api/users/new" should prefer a literal
// route over "/api/users/:id" if both exist), by scoring literal matches
// higher.
func (s *Server) match(method domain.Method, path string) *route {
	segments := splitPath(path)
	var best *route
	bestScore := -1
	for i := range s.routes {
		r := &s.routes[i]
		if r.method != method {
			continue
		}
		score, ok := scoreMatch(r.segments, segments)
		if ok && score > bestScore {
			best = r
			bestScore = score
		}
	}
	return best
}

// scoreMatch reports whether pattern (endpoint segments, possibly
// containing ":id") matches candidate (request segments), and a score
// that's higher the more segments matched literally (so a more specific
// route wins over a param-catching one when both would match).
func scoreMatch(pattern, candidate []string) (int, bool) {
	if len(pattern) != len(candidate) {
		return 0, false
	}
	score := 0
	for i := range pattern {
		if pattern[i] == candidate[i] {
			score++
			continue
		}
		if pattern[i] == ":id" {
			continue
		}
		return 0, false
	}
	return score, true
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}

func writeMockError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// writeCapturedExchange replays the exact status/headers/body of a
// previously captured example — the most faithful mock response
// available, since it's a real observed answer rather than a guess.
func writeCapturedExchange(w http.ResponseWriter, ex domain.Exchange) {
	for k, values := range ex.Response.Headers {
		for _, v := range values {
			w.Header().Add(k, v)
		}
	}
	status := ex.Response.StatusCode
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	_, _ = w.Write(ex.Response.Body)
}

// writeSyntheticResponse builds a minimal but honest response when no
// capture is available: the lowest declared 2xx status code (or 200 if
// none is known) and an empty JSON object body. It never invents field
// values a schema-less endpoint didn't actually declare
// (docs/11-risks-and-gaps.md's "never invent" discipline, applied here to
// mock bodies the same way it's applied to discovery).
func writeSyntheticResponse(w http.ResponseWriter, ep domain.Endpoint) {
	status := http.StatusOK
	if ep.Spec != nil {
		if codes, ok := ep.Spec.Responses.(map[string]any); ok {
			status = lowestSuccessCode(codes)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte("{}"))
}

func lowestSuccessCode(codes map[string]any) int {
	best := 0
	var keys []string
	for k := range codes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		n, err := strconv.Atoi(k)
		if err != nil || n < 200 || n >= 300 {
			continue
		}
		if best == 0 || n < best {
			best = n
		}
	}
	if best == 0 {
		return http.StatusOK
	}
	return best
}
