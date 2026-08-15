// Package runner executes a single HTTP exchange. It applies timeout and
// max-response-size limits, records timing, and does not retry or assert —
// those are testrunner's and assertions' jobs respectively
// (docs/04-interfaces.md section 4).
package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/security"
)

// DefaultTimeout is used when an HTTPRequest specifies no timeout.
const DefaultTimeout = 10 * time.Second

// Runner implements docs/04-interfaces.md section 4.
type Runner struct {
	client          *http.Client
	maxResponseSize int64
}

// Option configures a Runner.
type Option func(*Runner)

// WithMaxResponseSize overrides the default 5MB cap
// (docs/09-security.md section 4/10).
func WithMaxResponseSize(n int64) Option {
	return func(r *Runner) { r.maxResponseSize = n }
}

// WithHTTPClient overrides the underlying client (mainly for tests).
func WithHTTPClient(c *http.Client) Option {
	return func(r *Runner) { r.client = c }
}

// New builds a Runner. The underlying http.Client has no default timeout —
// per-request timeout is enforced via context instead, since Do also needs
// a bounded overall duration for the calling test.
func New(opts ...Option) *Runner {
	r := &Runner{
		client:          &http.Client{},
		maxResponseSize: security.DefaultMaxResponseSize,
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

// Do executes one HTTPRequest and returns the resulting Exchange. It never
// evaluates assertions and never retries; transport failures are returned
// as an error (callers decide whether that becomes a retry or a result).
func (r *Runner) Do(ctx context.Context, req domain.HTTPRequest) (domain.Exchange, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	fullURL, err := buildURL(req.URL, req.Query)
	if err != nil {
		return domain.Exchange{}, domain.NewConfigError("building request URL", err)
	}

	var bodyReader io.Reader
	if len(req.Body) > 0 {
		bodyReader = bytes.NewReader(req.Body)
	}

	httpReq, err := http.NewRequestWithContext(ctx, string(req.Method), fullURL, bodyReader)
	if err != nil {
		return domain.Exchange{}, domain.NewConfigError("constructing HTTP request", err)
	}
	if req.Headers != nil {
		httpReq.Header = req.Headers.Clone()
	}

	id := newExchangeID()
	start := time.Now()
	resp, err := r.client.Do(httpReq)
	end := time.Now()

	ex := domain.Exchange{
		ID:      id,
		Request: req,
		Timing: domain.Timing{
			Start:    start,
			End:      end,
			Duration: end.Sub(start),
		},
		Timestamp: start,
	}
	ex.Request.URL = fullURL

	if err != nil {
		ex.Err = err
		return ex, fmt.Errorf("%w: %s", domain.ErrTransport, err)
	}
	defer resp.Body.Close()

	respBody, truncated, err := readLimited(resp.Body, r.maxResponseSize)
	if err != nil {
		ex.Err = err
		return ex, fmt.Errorf("%w: reading response body: %s", domain.ErrTransport, err)
	}

	ex.Response = domain.HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       respBody,
		Truncated:  truncated,
	}
	return ex, nil
}

// buildURL merges any Query values onto the request URL. Values already in
// the URL's query string are preserved; req.Query values are appended.
func buildURL(raw string, query url.Values) (string, error) {
	if len(query) == 0 {
		return raw, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	existing := u.Query()
	for k, vs := range query {
		for _, v := range vs {
			existing.Add(k, v)
		}
	}
	u.RawQuery = existing.Encode()
	return u.String(), nil
}

// readLimited reads at most limit+1 bytes to detect truncation without
// buffering an unbounded response body in memory.
func readLimited(rc io.Reader, limit int64) ([]byte, bool, error) {
	if limit <= 0 {
		limit = security.DefaultMaxResponseSize
	}
	lr := io.LimitReader(rc, limit+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return nil, false, err
	}
	if int64(len(data)) > limit {
		return data[:limit], true, nil
	}
	return data, false, nil
}

func newExchangeID() domain.ExchangeID {
	return domain.ExchangeID(uuid.NewString())
}
