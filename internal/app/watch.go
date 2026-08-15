package app

import (
	"context"
	"fmt"
	"net/url"

	"github.com/sandeepv/apilens/internal/config"
	"github.com/sandeepv/apilens/internal/domain"
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
		_ = sessionFile.Append(stored)
		a.upsertObservedEndpoint(stored)
		out <- stored
	}
}

func (a *App) persistRegistrySnapshot() {
	endpoints := a.Registry.List(registry.Filter{})
	if len(endpoints) == 0 {
		return
	}
	_ = registry.SaveYAML(registryPath(a.ProjectDir), endpoints)
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
	path := requestPath(ex.Request.URL)
	_ = a.Registry.Upsert(domain.Endpoint{
		Method:        ex.Request.Method,
		Path:          path,
		Sources:       []string{"watch"},
		PrimarySource: "watch",
	})
}

// HistoryList backs `apilens history list`. It reads in-memory first
// (populated only while `watch` is running in this process); if empty, it
// falls back to the session JSONL file so a second terminal can see what
// the first terminal's `watch` captured (docs/08-proxy.md section 3).
func (a *App) HistoryList(limit int) []domain.Exchange {
	inMem := a.History.List(limit)
	if len(inMem) > 0 {
		return inMem
	}
	fromFile, _ := history.ReadAll(history.DefaultPath(a.ProjectDir))
	if limit > 0 && len(fromFile) > limit {
		fromFile = fromFile[len(fromFile)-limit:]
	}
	return fromFile
}

// HistoryGet backs `apilens history show <id>`, with the same in-memory
// then session-file fallback as HistoryList.
func (a *App) HistoryGet(displayID int) (domain.Exchange, bool) {
	if ex, ok := a.History.Get(displayID); ok {
		return ex, true
	}
	fromFile, _ := history.ReadAll(history.DefaultPath(a.ProjectDir))
	for _, ex := range fromFile {
		if int(ex.Display) == displayID {
			return ex, true
		}
	}
	return domain.Exchange{}, false
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
	return fmt.Sprintf("%s:%d", bind, port)
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
