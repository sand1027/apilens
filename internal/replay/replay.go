// Package replay implements docs/08-proxy.md section 8: reconstruct a
// stored exchange's request, apply overrides, and execute it through the
// shared runner. Replay always creates a NEW history entry — it never
// overwrites the one it replayed.
package replay

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/runner"
)

const maskValue = "********"

// Overrides mirrors docs/04-interfaces.md section 12. Headers/Query are
// set/replace; Unset removes a header entirely.
type Overrides struct {
	Method  *string
	URL     *string
	Headers map[string]string
	Unset   []string
	Query   map[string]string
	Body    []byte
}

// Service implements docs/04-interfaces.md section 12's replay.Service.
type Service struct {
	runner *runner.Runner
}

// New builds a replay Service using the given HTTP runner — the same
// runner every other live call (run, test, inspect --live) uses
// (docs/01-architecture.md section 5).
func New(r *runner.Runner) *Service {
	return &Service{runner: r}
}

// Replay reconstructs stored's request, applies ov, and executes it.
// Reconstruction refuses to send a request whose Authorization header is
// still masked ("Bearer ********") unless the caller's overrides replace
// it — sending the literal asterisks would silently look like an
// authenticated call while actually failing auth
// (docs/08-proxy.md section 8: "must fail with a clear error: use --env
// auth or --header. Do not send the literal asterisks").
func (s *Service) Replay(ctx context.Context, stored domain.Exchange, ov Overrides) (domain.Exchange, error) {
	req, err := buildRequest(stored, ov)
	if err != nil {
		return domain.Exchange{}, err
	}
	if err := checkNoMaskedSecrets(req.Headers); err != nil {
		return domain.Exchange{}, err
	}
	return s.runner.Do(ctx, req)
}

// buildRequest reconstructs domain.HTTPRequest from the stored exchange
// and applies overrides in the order: method, URL, headers set, headers
// unset, query, body.
func buildRequest(stored domain.Exchange, ov Overrides) (domain.HTTPRequest, error) {
	req := domain.HTTPRequest{
		Method:  stored.Request.Method,
		URL:     stored.Request.URL,
		Headers: stored.Request.Headers.Clone(),
		Body:    append([]byte(nil), stored.Request.Body...),
	}
	if req.Headers == nil {
		req.Headers = http.Header{}
	}

	if ov.Method != nil {
		req.Method = domain.NormalizeMethod(*ov.Method)
	}
	if ov.URL != nil {
		req.URL = *ov.URL
	}
	for k, v := range ov.Headers {
		req.Headers.Set(k, v)
	}
	for _, k := range ov.Unset {
		req.Headers.Del(k)
	}
	if len(ov.Query) > 0 {
		u, err := url.Parse(req.URL)
		if err != nil {
			return domain.HTTPRequest{}, domain.NewConfigError("parsing stored URL for replay", err)
		}
		q := u.Query()
		for k, v := range ov.Query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
		req.URL = u.String()
	}
	if ov.Body != nil {
		req.Body = ov.Body
	}
	return req, nil
}

// checkNoMaskedSecrets scans headers for the literal redaction placeholder
// and refuses to send it — see docs/08-proxy.md section 8.
func checkNoMaskedSecrets(h http.Header) error {
	for name, values := range h {
		for _, v := range values {
			if strings.Contains(v, maskValue) {
				return domain.NewSecurityError(
					"refusing to replay a masked header " + name +
						" — pass --header to supply a real value, or use environment auth")
			}
		}
	}
	return nil
}
