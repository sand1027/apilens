package app

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/sandeepv/apilens/internal/config"
	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/graphqlop"
	"github.com/sandeepv/apilens/internal/history"
	"github.com/sandeepv/apilens/internal/proxy"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/security"
)

// WatchOptions configures the Watch use case (docs/04-interfaces.md
// section 11, docs/05-cli.md "apilens watch").
type WatchOptions struct {
	Bind        string // overrides config.watch.bind:port when non-empty
	Port        int
	AllowRemote bool
	Upstream    string
	PathPrefix  string
	Host        string
	All         bool // --all: disable extension filtering
}

// WatchHandle is returned by Watch so the CLI can print the banner, read
// events, and stop cleanly on Ctrl+C.
//
// Events is the ONLY channel callers should read for display purposes. It
// is fed by pumpWatchEvents after history has assigned each exchange its
// DisplayID — the CLI must never read proxy.Proxy.Events() directly, since
// that channel has exactly one delivery per event and pumpWatchEvents is
// already consuming it (two consumers on one channel would silently split
// the traffic between them instead of both seeing every event).
type WatchHandle struct {
	SessionFile *history.SessionFile
	Addr        string
	Events      <-chan domain.Exchange
}

// Watch starts the local forward proxy, wires its capture events into the
// in-memory history ring, the session JSONL file (for cross-terminal
// replay/generate — docs/08-proxy.md section 3), and upserts observed
// endpoints into the registry (docs/07-discovery.md section 5: "Watch is
// not a file provider. It is a live source that upserts observed
// endpoints"). Start blocks until ctx is canceled — callers run it in a
// goroutine and cancel on Ctrl+C, matching docs/08-proxy.md section 3
// ("watch blocks the terminal").
func (a *App) Watch(ctx context.Context, opts WatchOptions) (*WatchHandle, error) {
	bind := resolveBind(a.Config.Watch, opts)

	redactor := security.New(security.Config{
		CaptureSensitiveHeaders: a.Config.Security.CaptureSensitiveHeaders,
		ExtraSensitiveHeaders:   a.Config.Security.SensitiveHeaders,
	})

	filter := proxy.Filter{
		PathPrefix:       firstNonEmpty(opts.PathPrefix, a.Config.Watch.PathPrefix),
		Host:             opts.Host,
		IgnoreExtensions: a.Config.Watch.IgnoreExtensions,
		All:              opts.All,
	}

	p := proxy.New(proxy.Options{
		Bind:            bind,
		AllowRemote:     opts.AllowRemote,
		Upstream:        opts.Upstream,
		MaxResponseSize: a.Config.MaxResponseSizeBytes(),
		Filter:          filter,
		Redactor:        redactor,
	})

	sessionPath := history.DefaultPath(a.ProjectDir)
	sessionFile, err := history.NewSessionFile(sessionPath)
	if err != nil {
		return nil, err
	}
	if err := history.SetActivePath(sessionPath); err != nil {
		fmt.Fprintf(os.Stderr, "apilens: could not publish history pointer: %v\n", err)
	}

	// Bind synchronously so a bad bind policy or a port already in use
	// fails the CLI command immediately rather than after printing a
	// banner that claims we're listening.
	if err := p.Bind(proxy.Options{Bind: bind, AllowRemote: opts.AllowRemote}); err != nil {
		return nil, err
	}

	go func() { _ = p.Serve(ctx) }()

	// pumpWatchEvents is the sole consumer of p.Events(). It enriches each
	// exchange with a DisplayID (via history.Append) before republishing
	// it on displayCh, which is what the CLI actually reads.
	displayCh := make(chan domain.Exchange, 64)
	go a.pumpWatchEvents(p, sessionFile, displayCh)

	return &WatchHandle{SessionFile: sessionFile, Addr: p.Addr(), Events: displayCh}, nil
}

// pumpWatchEvents drains the proxy's raw Events channel exactly once,
// assigns each exchange a DisplayID via history.Append, persists it to the
// session file, upserts the observed endpoint into the registry, and
// republishes the enriched exchange on out for the CLI to print. When the
// proxy's channel closes (ctx canceled — a clean stop), it persists the
// registry to disk once, so `list`/`inspect` in a later process can see
// what watch observed (docs/07-discovery.md section 8: "list and inspect
// should work ... in a new process"; section 10's "optionally persist
// registry.yaml" is exercised here rather than per-request, per
// docs/08-proxy.md section 10's latency budget: "do not write pretty YAML
// per request").
func (a *App) pumpWatchEvents(p *proxy.Proxy, sessionFile *history.SessionFile, out chan<- domain.Exchange) {
	defer close(out)
	defer a.persistRegistrySnapshot()
	for ex := range p.Events() {
		stored := a.History.Append(ex)
		if err := sessionFile.Append(stored); err != nil {
			fmt.Fprintf(os.Stderr, "apilens: could not write history: %v\n", err)
		}
		a.upsertObservedEndpoint(stored)
		out <- stored
	}
}

func (a *App) persistRegistrySnapshot() {
	endpoints := a.Registry.List(registry.Filter{})
	if len(endpoints) == 0 {
		return
	}
	if err := registry.SaveYAML(registryPath(a.ProjectDir), endpoints); err != nil {
		fmt.Fprintf(os.Stderr, "apilens: could not persist registry: %v\n", err)
	}
}

// upsertObservedEndpoint implements docs/07-discovery.md section 5:
// observed traffic enriches the registry but never overwrites a richer
// OpenAPI spec (registry.mergeUpsert already enforces that priority).
// Path params are not clustered in MVP (docs/11-risks-and-gaps.md R11:
// "Do not cluster in MVP — bad clustering is worse than verbose lists"),
// so "/api/users/1" stays literal, not "/api/users/:id".
func (a *App) upsertObservedEndpoint(ex domain.Exchange) {
	if ex.Request.URL == "" {
		return
	}
	method := ex.Request.Method
	path := requestPath(ex.Request.URL)
	var tags []string
	if _, op, ok := graphqlop.ParseHTTPBody(ex.Request.Body); ok {
		method = graphqlop.DisplayMethod(op.Type)
		path = graphqlop.RegistryPath(op.Type, op.PrimaryField())
		tags = []string{"graphql", op.Type}
	}
	_ = a.Registry.Upsert(domain.Endpoint{
		Method:        method,
		Path:          path,
		Sources:       []string{"watch"},
		PrimarySource: "watch",
		Tags:          tags,
	})
}

// HistoryList backs `apilens history list`. It reads in-memory first
// (populated only while `watch` is running in this process); if empty, it
// falls back to the session JSONL file so a second terminal can see what
// the first terminal's `watch` captured (docs/08-proxy.md section 3).
// A missing file is not an error; an unreadable file is.
func (a *App) HistoryList(limit int) ([]domain.Exchange, error) {
	inMem := a.History.List(limit)
	if len(inMem) > 0 {
		return inMem, nil
	}
	var fromFile []domain.Exchange
	for _, path := range history.SearchPaths(a.ProjectDir) {
		got, err := history.ReadAll(path)
		if err != nil {
			return nil, err
		}
		if len(got) > 0 {
			fromFile = got
			break
		}
	}
	if limit > 0 && len(fromFile) > limit {
		fromFile = fromFile[len(fromFile)-limit:]
	}
	return fromFile, nil
}

func (a *App) historyOrEmpty() []domain.Exchange {
	list, err := a.HistoryList(0)
	if err != nil {
		return nil
	}
	return list
}

// HistoryGet backs `apilens history show <id>`, with the same in-memory
// then session-file fallback as HistoryList.
func (a *App) HistoryGet(displayID int) (domain.Exchange, bool, error) {
	if ex, ok := a.History.Get(displayID); ok {
		return ex, true, nil
	}
	var found domain.Exchange
	ok := false
	for _, path := range history.SearchPaths(a.ProjectDir) {
		fromFile, err := history.ReadAll(path)
		if err != nil {
			return domain.Exchange{}, false, err
		}
		for _, item := range fromFile {
			if int(item.Display) == displayID {
				found = item
				ok = true
			}
		}
		if ok {
			break
		}
	}
	return found, ok, nil
}

func requestPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}

func resolveBind(cfg config.WatchConfig, opts WatchOptions) string {
	// A full host:port in Bind (including ":0" for an OS-assigned port)
	// wins so tests can request an ephemeral port without colliding on
	// the product default 8888.
	if opts.Bind != "" {
		if _, _, err := net.SplitHostPort(opts.Bind); err == nil {
			return opts.Bind
		}
	}
	bind := cfg.Bind
	port := cfg.Port
	if opts.Bind != "" {
		bind = opts.Bind
	}
	if opts.Port != 0 {
		port = opts.Port
	}
	if bind == "" {
		bind = "127.0.0.1"
	}
	if port == 0 {
		port = 8888
	}
	return net.JoinHostPort(bind, strconv.Itoa(port))
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
