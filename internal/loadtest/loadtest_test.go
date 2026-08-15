package loadtest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/environment"
	"github.com/sandeepv/apilens/internal/runner"
)

func testCase(url string, wantStatus int) domain.TestCase {
	s := wantStatus
	return domain.TestCase{
		Name:    "t",
		Request: domain.RequestTemplate{Method: "GET", URL: url},
		Assert:  domain.AssertionSpec{Status: &domain.StatusSpec{Equals: &s}},
	}
}

func TestRun_RequiresDurationOrIterations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	_, err := Run(context.Background(), []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{})
	if err == nil {
		t.Fatal("expected an error when neither Duration nor Iterations is set")
	}
}

func TestRun_RejectsChainedTests(t *testing.T) {
	tc := testCase("http://example.invalid", 200)
	tc.UsesChaining = true
	_, err := Run(context.Background(), []domain.TestCase{tc}, runner.New(), environment.New(), nil, Options{Iterations: 1})
	if err == nil {
		t.Fatal("expected an error rejecting a chained test from load mode")
	}
}

func TestRun_NoTestsIsConfigError(t *testing.T) {
	_, err := Run(context.Background(), nil, runner.New(), environment.New(), nil, Options{Iterations: 1})
	if err == nil {
		t.Fatal("expected an error for an empty test list")
	}
}

func TestRun_IterationsBoundStopsAtExactCount(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	report, err := Run(context.Background(), []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{
		Iterations: 37,
		Workers:    6,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.TotalRequests != 37 {
		t.Errorf("TotalRequests = %d, want exactly 37", report.TotalRequests)
	}
	if report.TotalErrors != 0 {
		t.Errorf("TotalErrors = %d, want 0", report.TotalErrors)
	}
}

func TestRun_DurationBoundStopsAfterElapsed(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	start := time.Now()
	report, err := Run(context.Background(), []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{
		Duration: 150 * time.Millisecond,
		Workers:  3,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected Run to return shortly after the 150ms duration bound, took %v", elapsed)
	}
	if report.TotalRequests == 0 {
		t.Error("expected at least some requests to complete within the duration")
	}
}

func TestRun_FailingAssertionCountsAsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(500) }))
	defer ts.Close()

	report, err := Run(context.Background(), []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{
		Iterations: 5,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if report.TotalErrors != 5 {
		t.Errorf("TotalErrors = %d, want 5 (every request returns 500, expected 200)", report.TotalErrors)
	}
	if report.ErrorRate() != 1.0 {
		t.Errorf("ErrorRate() = %v, want 1.0", report.ErrorRate())
	}
}

func TestRun_PerTestStatsMatchTheSingleTestCase(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	report, err := Run(context.Background(), []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{
		Iterations: 10,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.PerTest) != 1 {
		t.Fatalf("expected 1 per-test entry, got %d", len(report.PerTest))
	}
	if report.PerTest[0].Requests != 10 {
		t.Errorf("Requests = %d, want 10", report.PerTest[0].Requests)
	}
}

func TestRun_MultipleTestsCycleRoundRobin(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	tests := []domain.TestCase{testCase(ts.URL+"/a", 200), testCase(ts.URL+"/b", 200)}
	report, err := Run(context.Background(), tests, runner.New(), environment.New(), nil, Options{
		Iterations: 20,
		Workers:    1, // single worker makes round-robin cycling deterministic
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(report.PerTest) != 2 {
		t.Fatalf("expected 2 per-test entries, got %d", len(report.PerTest))
	}
	if report.PerTest[0].Requests != 10 || report.PerTest[1].Requests != 10 {
		t.Errorf("expected an even 10/10 split with a single round-robin worker, got %+v", report.PerTest)
	}
}

func TestRun_ContextCancelStopsPromptly(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) }))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	// Iterations is huge so only ctx cancellation stops it (proves Run
	// respects the caller's context, not just its own Duration option).
	_, err := Run(ctx, []domain.TestCase{testCase(ts.URL, 200)}, runner.New(), environment.New(), nil, Options{
		Iterations: 1_000_000,
		Workers:    2,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("expected ctx cancellation to stop Run quickly, took %v", elapsed)
	}
}
