package proxy

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

// handleForward implements docs/08-proxy.md section 10's latency budget:
// read request, tee body up to cap, RoundTrip, tee response, return. No
// pretty-printing or per-request YAML on the hot path.
func (p *Proxy) handleForward(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	reqBody, reqTruncated, err := readLimited(r.Body, p.opts.MaxResponseSize)
	if err != nil {
		http.Error(w, "reading request body: "+err.Error(), http.StatusBadGateway)
		return
	}

	targetURL := p.resolveTargetURL(r)
	outReq, err := http.NewRequest(r.Method, targetURL, bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "building forwarded request: "+err.Error(), http.StatusBadGateway)
		return
	}
	outReq.Header = r.Header.Clone()
	// A forward proxy must not forward this proxy's own hop-by-hop
	// header; the upstream never asked to know it went through us.
	outReq.Header.Del("Proxy-Connection")

	resp, err := p.client.Do(outReq)
	if err != nil {
		http.Error(w, "forwarding request: "+err.Error(), http.StatusBadGateway)
		p.maybeCapture(r, reqBody, reqTruncated, nil, nil, false, start, err)
		return
	}
	defer resp.Body.Close()

	respBody, respTruncated, err := readLimited(resp.Body, p.opts.MaxResponseSize)
	if err != nil {
		http.Error(w, "reading response body: "+err.Error(), http.StatusBadGateway)
		return
	}

	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)

	p.maybeCapture(r, reqBody, reqTruncated, resp, respBody, respTruncated, start, nil)
}

// maybeCapture applies the configured Filter, redacts, and emits a
// capture event. Filtered-out exchanges are simply dropped — this is a
// tap for developer visibility, not an audit log (docs/08-proxy.md
// section 6).
func (p *Proxy) maybeCapture(
	r *http.Request,
	reqBody []byte, reqTruncated bool,
	resp *http.Response, respBody []byte, respTruncated bool,
	start time.Time, transportErr error,
) {
	if !p.passesFilter(r) {
		return
	}

	end := time.Now()
	ex := domain.Exchange{
		Request: domain.HTTPRequest{
			Method:  domain.NormalizeMethod(r.Method),
			URL:     r.URL.String(),
			Headers: p.opts.Redactor.Headers(r.Header),
			Body:    p.opts.Redactor.Body(r.Header.Get("Content-Type"), reqBody),
		},
		Timing: domain.Timing{
			Start:    start,
			End:      end,
			Duration: end.Sub(start),
		},
		Timestamp: start,
		Redacted:  true,
	}
	if transportErr != nil {
		ex.Err = transportErr
	}
	if resp != nil {
		contentType := resp.Header.Get("Content-Type")
		ex.Response = domain.HTTPResponse{
			StatusCode: resp.StatusCode,
			Headers:    p.opts.Redactor.Headers(resp.Header),
			Body:       p.opts.Redactor.Body(contentType, respBody),
			Truncated:  respTruncated,
		}
	}
	// reqTruncated is intentionally unused beyond capping reqBody above:
	// domain.HTTPRequest has no Truncated field today (only responses do,
	// per docs/08-proxy.md section 4's Exchange record). A truncated
	// request body is simply capped, same as the runner's own behavior.

	select {
	case p.events <- ex:
	default:
		// A full events channel must never block the hot path — drop the
		// event rather than stall proxying (docs/08-proxy.md section 10).
	}
}

// passesFilter implements docs/08-proxy.md section 6: drop static asset
// noise by extension unless --all, and optionally restrict to a path
// prefix / host.
func (p *Proxy) passesFilter(r *http.Request) bool {
	if isApiLensUIRequest(r) {
		return false
	}
	f := p.opts.Filter
	if f.Host != "" && !strings.EqualFold(r.Host, f.Host) {
		return false
	}
	if !f.All && isBrowserUpdateHost(r.Host, r.URL.Host) {
		return false
	}
	if f.PathPrefix != "" && !strings.HasPrefix(r.URL.Path, f.PathPrefix) {
		return false
	}
	if !f.All {
		ext := strings.TrimPrefix(path.Ext(r.URL.Path), ".")
		for _, ig := range f.IgnoreExtensions {
			if strings.EqualFold(ext, ig) {
				return false
			}
		}
	}
	return true
}

// readLimited reads at most limit+1 bytes to detect truncation, mirroring
// internal/runner's approach so both paths behave identically for
// max_response_size (docs/09-security.md section 4/10).
func readLimited(rc io.Reader, limit int64) ([]byte, bool, error) {
	if limit <= 0 {
		limit = 5 * 1024 * 1024
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

// isApiLensUIRequest drops traffic to the local dashboard (default :4488)
// so overlay polls are not captured as API hits.
func isApiLensUIRequest(r *http.Request) bool {
	for _, host := range []string{r.Host, r.URL.Host} {
		if host == "" {
			continue
		}
		h, port, err := net.SplitHostPort(host)
		if err != nil {
			continue
		}
		if port != "4488" {
			continue
		}
		if isLoopbackName(h) {
			return true
		}
	}
	return false
}

func isLoopbackName(h string) bool {
	h = strings.Trim(h, "[]")
	if strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(h)
	return ip != nil && ip.IsLoopback()
}

// isBrowserUpdateHost drops Chrome component-update noise (gvt1, googleapis)
// so watch stays readable. --all captures it.
func isBrowserUpdateHost(hosts ...string) bool {
	for _, host := range hosts {
		h := strings.ToLower(host)
		if i := strings.IndexByte(h, ':'); i >= 0 {
			h = h[:i]
		}
		for _, s := range []string{
			"googleapis.com", "gvt1.com", "google.com", "gstatic.com",
			"googleusercontent.com", "google-analytics.com",
		} {
			if h == s || strings.HasSuffix(h, "."+s) {
				return true
			}
		}
	}
	return false
}
