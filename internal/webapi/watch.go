package webapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

// watchController manages the single running watch session (if any) and
// fans its Events channel out to every subscribed browser tab — the
// Engine only gives one channel per Watch call, but the Runtime Monitor
// tab (plan.md v6) may be open in more than one tab, or reloaded, so this
// package-level broadcaster is dashboard-specific plumbing, not a change
// to the Engine contract.
type watchController struct {
	engine apilens.Engine

	mu     sync.Mutex
	cancel context.CancelFunc
	addr   string
	subs   map[chan domain.Exchange]struct{}
}

func newWatchController(engine apilens.Engine) *watchController {
	return &watchController{engine: engine, subs: make(map[chan domain.Exchange]struct{})}
}

// startWatchRequest is the POST /api/watch/start body, mirroring
// apilens.WatchOptions / `apilens watch` flags.
type startWatchRequest struct {
	Bind        string `json:"bind"`
	Port        int    `json:"port"`
	AllowRemote bool   `json:"allowRemote"`
	Upstream    string `json:"upstream"`
	PathPrefix  string `json:"pathPrefix"`
	Host        string `json:"host"`
	All         bool   `json:"all"`
}

// handleStart starts the local proxy if one isn't already running.
// POST /api/watch/start
func (c *watchController) handleStart(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	if c.cancel != nil {
		c.mu.Unlock()
		writeError(w, domain.NewConfigError("watch is already running — stop it first", nil))
		return
	}
	c.mu.Unlock()

	var req startWatchRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}

	// Use a background context (not r.Context()) since watch must keep
	// running after this HTTP request completes — it's stopped
	// explicitly via /api/watch/stop, not by the request that started it.
	ctx, cancel := context.WithCancel(context.Background())
	session, err := c.engine.Watch(ctx, apilens.WatchOptions{
		Bind:        req.Bind,
		Port:        req.Port,
		AllowRemote: req.AllowRemote,
		Upstream:    req.Upstream,
		PathPrefix:  req.PathPrefix,
		Host:        req.Host,
		All:         req.All,
	})
	if err != nil {
		cancel()
		writeError(w, err)
		return
	}

	c.mu.Lock()
	c.cancel = cancel
	c.addr = session.Addr
	c.mu.Unlock()

	go c.pump(session.Events)

	writeJSON(w, http.StatusOK, struct {
		Addr string `json:"addr"`
	}{Addr: session.Addr})
}

// pump is the sole reader of the Engine's Events channel (mirroring
// internal/app's own single-consumer rule for the proxy's raw channel —
// see the v3 concurrency fix) and fans each exchange out to every current
// subscriber without blocking on a slow one.
func (c *watchController) pump(events <-chan domain.Exchange) {
	for ex := range events {
		c.mu.Lock()
		for sub := range c.subs {
			select {
			case sub <- ex:
			default:
				// A slow/stuck subscriber must never stall the whole
				// broadcast (same non-blocking-send principle as the
				// proxy's own Events channel).
			}
		}
		c.mu.Unlock()
	}
	c.mu.Lock()
	c.cancel = nil
	c.addr = ""
	c.mu.Unlock()
}

// handleStop cancels the running watch, if any. POST /api/watch/stop
func (c *watchController) handleStop(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel == nil {
		writeError(w, domain.NewConfigError("watch is not running", nil))
		return
	}
	cancel()
	writeJSON(w, http.StatusOK, struct {
		Stopped bool `json:"stopped"`
	}{Stopped: true})
}

// handleStatus reports whether watch is running and its bound address.
// GET /api/watch/status
func (c *watchController) handleStatus(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	running := c.cancel != nil
	addr := c.addr
	c.mu.Unlock()
	writeJSON(w, http.StatusOK, struct {
		Running bool   `json:"running"`
		Addr    string `json:"addr,omitempty"`
	}{Running: running, Addr: addr})
}

// handleEvents streams captured exchanges as Server-Sent Events for the
// Runtime Monitor tab. GET /api/watch/events
func (c *watchController) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, domain.NewConfigError("streaming not supported", nil))
		return
	}

	sub := make(chan domain.Exchange, 32)
	c.mu.Lock()
	c.subs[sub] = struct{}{}
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.subs, sub)
		c.mu.Unlock()
		close(sub)
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case ex := <-sub:
			data, err := json.Marshal(ex)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}
