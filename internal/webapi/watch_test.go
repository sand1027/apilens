package webapi

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

func TestWatchStart_ReturnsAddrAndSetsRunning(t *testing.T) {
	// Keep the Events channel open (never closed) for the duration of the
	// test so the pump goroutine doesn't race the status check below.
	events := make(chan domain.Exchange)
	defer close(events)
	eng := &fakeEngine{watchFn: func(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error) {
		return &apilens.WatchSession{Addr: "127.0.0.1:9999", Events: events}, nil
	}}
	s := New(eng, nil)

	rec := doRequest(t, s, "POST", "/api/watch/start", "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("start status = %d, body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "127.0.0.1:9999") {
		t.Errorf("start response = %q, want it to include the bound addr", rec.Body.String())
	}

	statusRec := doRequest(t, s, "GET", "/api/watch/status", "")
	if !strings.Contains(statusRec.Body.String(), `"running":true`) {
		t.Errorf("status = %q, want running:true", statusRec.Body.String())
	}
	if !strings.Contains(statusRec.Body.String(), "127.0.0.1:9999") {
		t.Errorf("status = %q, want it to include the bound addr", statusRec.Body.String())
	}
}

func TestWatchStart_RejectsSecondStartWhileRunning(t *testing.T) {
	blockCh := make(chan struct{})
	eng := &fakeEngine{watchFn: func(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error) {
		events := make(chan domain.Exchange)
		go func() { <-blockCh; close(events) }()
		return &apilens.WatchSession{Addr: "127.0.0.1:9999", Events: events}, nil
	}}
	s := New(eng, nil)
	defer close(blockCh)

	rec1 := doRequest(t, s, "POST", "/api/watch/start", "{}")
	if rec1.Code != http.StatusOK {
		t.Fatalf("first start status = %d", rec1.Code)
	}
	rec2 := doRequest(t, s, "POST", "/api/watch/start", "{}")
	if rec2.Code != http.StatusBadRequest {
		t.Errorf("second start status = %d, want 400 (already running)", rec2.Code)
	}
}

func TestWatchStop_WithoutRunningReturns400(t *testing.T) {
	eng := &fakeEngine{}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/watch/stop", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestWatchEvents_StreamsBroadcastExchange(t *testing.T) {
	events := make(chan domain.Exchange, 1)
	eng := &fakeEngine{watchFn: func(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error) {
		return &apilens.WatchSession{Addr: "127.0.0.1:9999", Events: events}, nil
	}}
	s := New(eng, nil)

	if rec := doRequest(t, s, "POST", "/api/watch/start", "{}"); rec.Code != http.StatusOK {
		t.Fatalf("start status = %d", rec.Code)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/watch/events", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/watch/events: %v", err)
	}
	defer resp.Body.Close()

	// Give the SSE handler a moment to register its subscription before
	// we publish, otherwise the event could be sent before anyone is
	// listening (the broadcaster is deliberately non-blocking/lossy for
	// slow subscribers, not a queue).
	time.Sleep(50 * time.Millisecond)
	events <- domain.Exchange{Request: domain.HTTPRequest{Method: "GET", URL: "/observed"}}

	scanner := bufio.NewScanner(resp.Body)
	found := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data:") && strings.Contains(line, "/observed") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected the SSE stream to deliver the published exchange")
	}
}

func TestWatchEvents_MultipleSubscribersEachReceiveTheEvent(t *testing.T) {
	events := make(chan domain.Exchange, 1)
	eng := &fakeEngine{watchFn: func(ctx context.Context, opts apilens.WatchOptions) (*apilens.WatchSession, error) {
		return &apilens.WatchSession{Addr: "127.0.0.1:9999", Events: events}, nil
	}}
	s := New(eng, nil)
	if rec := doRequest(t, s, "POST", "/api/watch/start", "{}"); rec.Code != http.StatusOK {
		t.Fatalf("start status = %d", rec.Code)
	}

	ts := httptest.NewServer(s)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	subscribe := func() *http.Response {
		req, _ := http.NewRequestWithContext(ctx, "GET", ts.URL+"/api/watch/events", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		return resp
	}

	resp1 := subscribe()
	defer resp1.Body.Close()
	resp2 := subscribe()
	defer resp2.Body.Close()

	time.Sleep(50 * time.Millisecond)
	events <- domain.Exchange{Request: domain.HTTPRequest{Method: "GET", URL: "/fanout-test"}}

	for i, resp := range []*http.Response{resp1, resp2} {
		scanner := bufio.NewScanner(resp.Body)
		found := false
		for scanner.Scan() {
			if strings.Contains(scanner.Text(), "/fanout-test") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("subscriber %d did not receive the broadcast event", i)
		}
	}
}
