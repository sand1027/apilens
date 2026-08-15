package webapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/pkg/apilens"
)

func doRequest(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var reqBody *strings.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	} else {
		reqBody = strings.NewReader("")
	}
	req := httptest.NewRequest(method, path, reqBody)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestHandleListEndpoints_ReturnsEmptyArrayNotNull(t *testing.T) {
	eng := &fakeEngine{listFn: func(f apilens.EndpointFilter) []apilens.Endpoint { return nil }}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/endpoints", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body.String())
	}
}

func TestHandleListEndpoints_PassesFilterFromQuery(t *testing.T) {
	var gotFilter apilens.EndpointFilter
	eng := &fakeEngine{listFn: func(f apilens.EndpointFilter) []apilens.Endpoint {
		gotFilter = f
		return nil
	}}
	s := New(eng, nil)
	doRequest(t, s, "GET", "/api/endpoints?method=GET&tag=users", "")
	if gotFilter.Method != "GET" || gotFilter.Tag != "users" {
		t.Errorf("filter = %+v", gotFilter)
	}
}

func TestHandleDiscover_ReturnsResultJSON(t *testing.T) {
	eng := &fakeEngine{discoverFn: func(ctx context.Context, opts apilens.DiscoverOptions) (*apilens.DiscoverResult, error) {
		return &apilens.DiscoverResult{Endpoints: []apilens.Endpoint{{Method: "GET", Path: "/x"}}}, nil
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/discover", "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var out apilens.DiscoverResult
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(out.Endpoints) != 1 {
		t.Errorf("Endpoints = %+v", out.Endpoints)
	}
}

func TestHandleInspect_MissingRefReturns400(t *testing.T) {
	eng := &fakeEngine{}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/inspect", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleInspect_NotFoundReturns404(t *testing.T) {
	eng := &fakeEngine{inspectFn: func(ctx context.Context, ref apilens.InspectRef) (*apilens.Inspection, error) {
		return nil, domain.NewNotFoundError("no such endpoint")
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/inspect?ref=/nope", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
	var errBody apiError
	if err := json.Unmarshal(rec.Body.Bytes(), &errBody); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if errBody.Error == "" {
		t.Error("expected a non-empty error message")
	}
}

func TestHandleRun_ReturnsReport(t *testing.T) {
	eng := &fakeEngine{runFn: func(ctx context.Context, filter apilens.RunFilter) (*apilens.Report, error) {
		return &apilens.Report{Counts: domain.Counts{Tests: 2, Passed: 2}}, nil
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/run", `{"tag":"smoke"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var report apilens.Report
	if err := json.Unmarshal(rec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if report.Counts.Tests != 2 {
		t.Errorf("Counts = %+v", report.Counts)
	}
}

func TestHandleRun_ConfigErrorReturns400(t *testing.T) {
	eng := &fakeEngine{runFn: func(ctx context.Context, filter apilens.RunFilter) (*apilens.Report, error) {
		return nil, domain.NewConfigError("no tests matched", nil)
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/run", "{}")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleHistoryList_ReturnsEmptyArrayNotNull(t *testing.T) {
	eng := &fakeEngine{historyFn: func(limit int) ([]domain.Exchange, error) { return nil, nil }}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/history", "")
	if strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Errorf("body = %q, want []", rec.Body.String())
	}
}

func TestHandleHistoryList_EncodesTransportError(t *testing.T) {
	eng := &fakeEngine{historyFn: func(limit int) ([]domain.Exchange, error) {
		return []domain.Exchange{{
			Display: 3,
			Request: domain.HTTPRequest{Method: "GET", URL: "http://localhost:3000/graphql"},
			Err:     errors.New("dial tcp: connection refused"),
		}}, nil
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/history", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "connection refused") {
		t.Errorf("body = %s", rec.Body.String())
	}
}

func TestHandleHistoryGet_NotFoundReturns404(t *testing.T) {
	eng := &fakeEngine{historyGetFn: func(id int) (*domain.Exchange, bool, error) { return nil, false, nil }}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/history/42", "")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestHandleHistoryGet_InvalidIDReturns400(t *testing.T) {
	eng := &fakeEngine{}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/history/not-a-number", "")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleReplay_SecurityErrorReturns403(t *testing.T) {
	eng := &fakeEngine{replayFn: func(ctx context.Context, id int, ov apilens.ReplayOverrides) (*domain.Exchange, error) {
		return nil, domain.NewSecurityError("refusing to replay a masked header")
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/replay/1", "{}")
	if rec.Code != http.StatusForbidden {
		t.Errorf("status = %d, want 403", rec.Code)
	}
}

func TestHandleGenerate_ReturnsGeneratedTest(t *testing.T) {
	eng := &fakeEngine{generateFn: func(id int, opts apilens.GenerateOptions) (*apilens.GeneratedTest, error) {
		return &apilens.GeneratedTest{Path: "generated/get-x.yaml"}, nil
	}}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/generate/1", "{}")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var out apilens.GeneratedTest
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Path != "generated/get-x.yaml" {
		t.Errorf("Path = %q", out.Path)
	}
}

func TestHandleListEnvironments_ReturnsCurrentAndItems(t *testing.T) {
	eng := &fakeEngine{
		currentEnvFn:   func() domain.Environment { return domain.Environment{Name: "local"} },
		environmentsFn: func() []domain.Environment { return []domain.Environment{{Name: "local"}, {Name: "staging"}} },
	}
	s := New(eng, nil)
	rec := doRequest(t, s, "GET", "/api/environments", "")
	var out struct {
		Current string               `json:"current"`
		Items   []domain.Environment `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Current != "local" || len(out.Items) != 2 {
		t.Errorf("out = %+v", out)
	}
}

func TestHandleUseEnvironment_MissingNameReturns400(t *testing.T) {
	eng := &fakeEngine{}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/environments/use", "{}")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", rec.Code)
	}
}

func TestHandleUseEnvironment_UnknownEnvReturns404(t *testing.T) {
	eng := &fakeEngine{useEnvFn: func(name string) error { return domain.NewNotFoundError("unknown environment") }}
	s := New(eng, nil)
	rec := doRequest(t, s, "POST", "/api/environments/use", `{"name":"nope"}`)
	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

func TestFrontendFallback_ServesEmbeddedHandlerForUnknownPaths(t *testing.T) {
	frontend := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("frontend-placeholder"))
	})
	eng := &fakeEngine{}
	s := New(eng, frontend)
	rec := doRequest(t, s, "GET", "/some/spa/route", "")
	if rec.Body.String() != "frontend-placeholder" {
		t.Errorf("body = %q, want the frontend handler's output", rec.Body.String())
	}
}

func TestWriteError_MapsErrorTypesToStatusCodes(t *testing.T) {
	cases := []struct {
		err    error
		status int
	}{
		{domain.NewNotFoundError("x"), http.StatusNotFound},
		{domain.NewConfigError("x", nil), http.StatusBadRequest},
		{domain.NewSecurityError("x"), http.StatusForbidden},
		{domain.ErrNotImplemented, http.StatusNotImplemented},
		{errors.New("unknown"), http.StatusInternalServerError},
	}
	for _, c := range cases {
		rec := httptest.NewRecorder()
		writeError(rec, c.err)
		if rec.Code != c.status {
			t.Errorf("writeError(%v) status = %d, want %d", c.err, rec.Code, c.status)
		}
	}
}

func TestLoopbackCORS_AllowsLocalhostOrigin(t *testing.T) {
	eng := &fakeEngine{historyFn: func(limit int) ([]domain.Exchange, error) { return nil, nil }}
	s := New(eng, nil)
	req := httptest.NewRequest("GET", "/api/history", nil)
	req.Header.Set("Origin", "http://localhost:3001")
	rec := httptest.NewRecorder()
	s.ServeHTTP(rec, req)
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3001" {
		t.Errorf("CORS origin = %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
	if rec.Header().Get("Access-Control-Allow-Private-Network") != "true" {
		t.Errorf("PNA header = %q", rec.Header().Get("Access-Control-Allow-Private-Network"))
	}
}

func TestWidgetJS_IsServed(t *testing.T) {
	s := New(&fakeEngine{}, nil)
	rec := doRequest(t, s, "GET", "/widget.js", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "apilens") && !strings.Contains(rec.Body.String(), "Live hits") {
		t.Errorf("widget.js body missing overlay: %s", rec.Body.String()[:min(200, rec.Body.Len())])
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "javascript") {
		t.Errorf("Content-Type = %q", rec.Header().Get("Content-Type"))
	}
}
