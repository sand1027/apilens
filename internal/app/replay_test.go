package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
	"github.com/sandeepv/apilens/internal/replay"
)

func TestReplay_LooksUpHistoryAndAppendsNewEntry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer ts.Close()

	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	original := a.History.Append(domain.Exchange{
		Request: domain.HTTPRequest{Method: "GET", URL: ts.URL + "/x"},
	})

	ex, err := a.Replay(context.Background(), int(original.Display), replay.Overrides{})
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if ex.Display == original.Display {
		t.Error("expected replay to create a new history entry, not reuse the original's Display ID")
	}
	if ex.Response.StatusCode != 200 {
		t.Errorf("StatusCode = %d", ex.Response.StatusCode)
	}
}

func TestReplay_UnknownIDReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = a.Replay(context.Background(), 999, replay.Overrides{})
	if err == nil {
		t.Fatal("expected not-found error for an unknown history id")
	}
}

func TestGenerate_WritesFromHistoryEntry(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	stored := a.History.Append(domain.Exchange{
		Request:  domain.HTTPRequest{Method: "GET", URL: "http://localhost:5000/api/things"},
		Response: domain.HTTPResponse{StatusCode: 200},
	})

	g, err := a.Generate(int(stored.Display), generate.Options{})
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if g.Path == "" {
		t.Error("expected a non-empty generated path")
	}
}

func TestGenerate_UnknownIDReturnsNotFound(t *testing.T) {
	dir := t.TempDir()
	a, err := New(Options{ProjectDir: dir})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = a.Generate(999, generate.Options{})
	if err == nil {
		t.Fatal("expected not-found error for an unknown history id")
	}
}
