// Package proxy implements the local forward proxy from docs/08-proxy.md.
// It is a tap, not a debugger: it reads a request, forwards it unchanged,
// tees the response, and emits a capture event. No MITM, no rewriting, no
// mocking (docs/08-proxy.md section 11).
package proxy

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/security"
)

// Filter narrows which exchanges are captured/emitted
// (docs/08-proxy.md section 6).
type Filter struct {
	// PathPrefix, if set, only captures requests whose path has this
	// prefix (e.g. "/api").
	PathPrefix string
	// Host, if set, only captures requests to this host.
	Host string
	// IgnoreExtensions drops requests whose path ends in one of these
	// extensions (without the leading dot), e.g. "js", "css", "png".
	// Defaults to docs/08-proxy.md section 6's list when nil.
	IgnoreExtensions []string
	// All disables extension filtering entirely (the --all flag).
	All bool
}

// DefaultIgnoreExtensions matches docs/08-proxy.md section 6.
var DefaultIgnoreExtensions = []string{"js", "css", "map", "png", "jpg", "svg", "woff2"}

// Options configures Start (docs/04-interfaces.md section 11).
type Options struct {
	// Bind is the listen address, default "127.0.0.1:8888".
	Bind string
	// AllowRemote permits a non-loopback Bind (docs/09-security.md
	// section 5); the bind-policy check still runs either way.
	AllowRemote bool
	// Upstream, if set, puts the proxy in reverse mode: every request is
	// forwarded to this base URL regardless of its own Host header
	// (docs/08-proxy.md section 5, the --upstream reverse-proxy mode).
	Upstream string
	// MaxResponseSize caps captured (and forwarded) response body bytes;
	// <=0 uses security.DefaultMaxResponseSize.
	MaxResponseSize int64
	// Filter narrows what gets captured.
	Filter Filter
	// Redactor masks sensitive headers/body content before an exchange is
	// emitted, stored, or written to the session file
	// (docs/09-security.md section 3: redaction runs before both display
	// and persistence).
	Redactor *security.Redactor
}

// Proxy implements docs/04-interfaces.md section 11.
type Proxy struct {
	opts     Options
	events   chan domain.Exchange
	server   *http.Server
	client   *http.Client
	addr     string
	listener net.Listener
}

// New builds a Proxy. It does not start listening — call Start.
func New(opts Options) *Proxy {
	if opts.Bind == "" {
		opts.Bind = "127.0.0.1:8888"
	}
	if opts.MaxResponseSize <= 0 {
		opts.MaxResponseSize = security.DefaultMaxResponseSize
	}
	if opts.Redactor == nil {
		opts.Redactor = security.New(security.Config{})
	}
	if len(opts.Filter.IgnoreExtensions) == 0 && !opts.Filter.All {
		opts.Filter.IgnoreExtensions = DefaultIgnoreExtensions
	}
	return &Proxy{
		opts:   opts,
		events: make(chan domain.Exchange, 64),
		client: &http.Client{
			// The proxy itself must not follow redirects on the client's
			// behalf — it forwards exactly what the upstream/target
			// returns (docs/08-proxy.md section 1: "tap, not a debugger
			// that mutates traffic").
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Events returns the channel of captured exchanges. Callers (watch's
// terminal printer, history, registry upsert) all subscribe here.
func (p *Proxy) Events() <-chan domain.Exchange { return p.events }

// Addr returns the actual bound address once Start has succeeded.
func (p *Proxy) Addr() string { return p.addr }

// Bind validates the bind policy and opens the listener, returning
// immediately with any error (bad bind policy, port in use). Callers that
// need to know synchronously whether the proxy is actually listening
// (e.g. printing the watch banner, or failing the CLI command) call Bind
// before Serve.
func (p *Proxy) Bind(opts Options) error {
	if opts.Bind != "" {
		p.opts.Bind = opts.Bind
	}
	allowRemote := opts.AllowRemote || p.opts.AllowRemote
	if err := security.ValidateListenAddr(p.opts.Bind, allowRemote); err != nil {
		return err
	}

	ln, err := net.Listen("tcp", p.opts.Bind)
	if err != nil {
		return domain.NewSecurityError("binding proxy: " + err.Error())
	}
	p.addr = ln.Addr().String()
	p.listener = ln
	return nil
}

// Serve runs the proxy's HTTP handler over the listener opened by Bind
// until ctx is canceled. It blocks — callers run it in the watch
// command's goroutine or foreground per docs/08-proxy.md section 3
// ("watch blocks the terminal"). Bind must be called first.
func (p *Proxy) Serve(ctx context.Context) error {
	if p.listener == nil {
		return domain.NewSecurityError("proxy Serve called before Bind")
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.HandlerFunc(p.handle))
	p.server = &http.Server{Handler: mux}

	errCh := make(chan error, 1)
	go func() {
		errCh <- p.server.Serve(p.listener)
	}()

	select {
	case <-ctx.Done():
		_ = p.server.Close()
		close(p.events)
		return nil
	case err := <-errCh:
		close(p.events)
		if err != nil && err != http.ErrServerClosed {
			return domain.NewSecurityError("proxy serve error: " + err.Error())
		}
		return nil
	}
}

// Start implements docs/04-interfaces.md section 11's Proxy interface by
// composing Bind then Serve. Most callers that need to detect a bind
// failure synchronously should call Bind + Serve directly instead (see
// internal/app.Watch); Start exists so Proxy satisfies the documented
// single-method contract for simple embedders.
func (p *Proxy) Start(ctx context.Context, opts Options) error {
	if err := p.Bind(opts); err != nil {
		return err
	}
	return p.Serve(ctx)
}

// handle dispatches CONNECT (HTTPS tunnel, no MITM) vs. plain HTTP forward
// proxying.
func (p *Proxy) handle(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/__apilens__/proxy.pac" {
		p.handlePAC(w)
		return
	}
	if r.Method == http.MethodConnect {
		p.handleConnect(w, r)
		return
	}
	p.handleForward(w, r)
}

// PACScript returns a PAC that sends only loopback traffic through the
// forward proxy. Chrome still needs --proxy-bypass-list=<-loopback> or it
// ignores this for localhost (the default bypass list wins over PAC).
func PACScript(proxyAddr string) string {
	return fmt.Sprintf(`function FindProxyForURL(url, host) {
  host = (host || "").toLowerCase();
  if (shExpMatch(url, "*://127.0.0.1:4488/*") || shExpMatch(url, "*://localhost:4488/*") || shExpMatch(url, "*://[::1]:4488/*")) {
    return "DIRECT";
  }
  if (host === "localhost" || host === "127.0.0.1" || host === "::1" || host === "[::1]" || shExpMatch(host, "127.*")) {
    return "PROXY %s";
  }
  return "DIRECT";
}
`, proxyAddr)
}

// handlePAC is used by `apilens watch --browser`. Chrome ignores HTTP_PROXY
// for localhost; a PAC that names localhost explicitly plus the loopback
// bypass override is how we capture GraphQL without changing the app URL.
func (p *Proxy) handlePAC(w http.ResponseWriter) {
	proxy := p.addr
	if proxy == "" {
		proxy = p.opts.Bind
	}
	w.Header().Set("Content-Type", "application/x-ns-proxy-autoconfig")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, PACScript(proxy))
}

// handleConnect tunnels HTTPS traffic byte-for-byte without inspecting it
// (docs/08-proxy.md section 5: "HTTPS CONNECT: Tunnel only — method/path
// inside TLS not visible"). No capture event is emitted for tunneled
// traffic in MVP.
func (p *Proxy) handleConnect(w http.ResponseWriter, r *http.Request) {
	destConn, err := net.DialTimeout("tcp", r.Host, 10*time.Second)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer destConn.Close()

	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}
	clientConn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	defer clientConn.Close()

	if _, err := clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n")); err != nil {
		return
	}

	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(destConn, clientConn); done <- struct{}{} }()
	go func() { _, _ = io.Copy(clientConn, destConn); done <- struct{}{} }()
	<-done
}

// resolveTargetURL implements forward-proxy vs. reverse-proxy (--upstream)
// mode from docs/08-proxy.md section 5.
func (p *Proxy) resolveTargetURL(r *http.Request) string {
	if p.opts.Upstream != "" {
		return strings.TrimSuffix(p.opts.Upstream, "/") + r.URL.Path + questionMark(r.URL.RawQuery)
	}
	return r.URL.String()
}

func questionMark(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	return "?" + rawQuery
}
