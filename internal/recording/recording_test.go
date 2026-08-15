package recording

import (
	"net/http"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func jsonExchange(method, url string, status int, reqBody, respBody string) domain.Exchange {
	h := http.Header{"Content-Type": []string{"application/json"}}
	ex := domain.Exchange{
		Request: domain.HTTPRequest{Method: domain.Method(method), URL: url, Headers: h},
		Response: domain.HTTPResponse{
			StatusCode: status, Headers: h,
			Body: []byte(respBody),
		},
	}
	if reqBody != "" {
		ex.Request.Body = []byte(reqBody)
	}
	return ex
}

func TestSession_EmptyInputIsConfigError(t *testing.T) {
	_, err := Session(nil)
	if err == nil {
		t.Fatal("expected an error for an empty exchange list")
	}
}

func TestSession_SingleStepHasNoChainingReference(t *testing.T) {
	ex := jsonExchange("GET", "http://x/health", 200, "", `{"status":"ok"}`)
	steps, err := Session([]domain.Exchange{ex})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(steps))
	}
	if steps[0].Test.UsesChaining {
		t.Error("a single step has nothing earlier to chain from")
	}
	if steps[0].Test.Version != 2 {
		t.Errorf("Version = %d, want 2", steps[0].Test.Version)
	}
}

func TestSession_CorrelatesLaterURLSegmentToEarlierResponseID(t *testing.T) {
	login := jsonExchange("POST", "http://x/api/users", 201,
		`{"name":"ada"}`, `{"data":{"id":7,"name":"ada"}}`)
	fetch := jsonExchange("GET", "http://x/api/users/7", 200, "", `{"data":{"id":7}}`)

	steps, err := Session([]domain.Exchange{login, fetch})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if len(steps) != 2 {
		t.Fatalf("expected 2 steps, got %d", len(steps))
	}

	fetchStep := steps[1]
	if !fetchStep.Test.UsesChaining {
		t.Error("expected the fetch step to be flagged as using chaining")
	}
	wantURL := "{{base_url}}/api/users/{{responses." + steps[0].ID + ".body.data.id}}"
	if fetchStep.Test.Request.URL != wantURL {
		t.Errorf("URL = %q, want %q", fetchStep.Test.Request.URL, wantURL)
	}
}

func TestSession_CorrelatesJSONBodyLeafToEarlierResponse(t *testing.T) {
	login := jsonExchange("POST", "http://x/api/users", 201,
		`{"name":"ada"}`, `{"data":{"id":7,"token":"tok-abc"}}`)
	action := jsonExchange("POST", "http://x/api/orders", 201,
		`{"user_id":"7","auth":"tok-abc"}`, `{"data":{"id":99}}`)

	steps, err := Session([]domain.Exchange{login, action})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	body, ok := steps[1].Test.Request.Body.JSON.(map[string]any)
	if !ok {
		t.Fatalf("expected a JSON body map, got %T", steps[1].Test.Request.Body.JSON)
	}
	wantUserID := "{{responses." + steps[0].ID + ".body.data.id}}"
	wantAuth := "{{responses." + steps[0].ID + ".body.data.token}}"
	if body["user_id"] != wantUserID {
		t.Errorf("user_id = %v, want %v", body["user_id"], wantUserID)
	}
	if body["auth"] != wantAuth {
		t.Errorf("auth = %v, want %v", body["auth"], wantAuth)
	}
	if !steps[1].Test.UsesChaining {
		t.Error("expected the second step to be flagged as using chaining")
	}
}

func TestSession_NeverCorrelatesUnrelatedValues(t *testing.T) {
	// "ada" in the second request's body happens to be a common name but
	// was never seen in the first response — must NOT be rewritten as a
	// chaining reference just because it looks plausible.
	login := jsonExchange("POST", "http://x/api/users", 201,
		`{"name":"bob"}`, `{"data":{"id":7,"name":"bob"}}`)
	unrelated := jsonExchange("POST", "http://x/api/orders", 201,
		`{"customer":"ada"}`, `{"data":{"id":1}}`)

	steps, err := Session([]domain.Exchange{login, unrelated})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	body := steps[1].Test.Request.Body.JSON.(map[string]any)
	if body["customer"] != "ada" {
		t.Errorf("customer = %v, want the literal unmodified value %q", body["customer"], "ada")
	}
	if steps[1].Test.UsesChaining {
		t.Error("no genuine match exists — UsesChaining should be false")
	}
}

func TestSession_DuplicateEndpointGetsDisambiguatedID(t *testing.T) {
	a := jsonExchange("GET", "http://x/health", 200, "", `{"status":"ok"}`)
	b := jsonExchange("GET", "http://x/health", 200, "", `{"status":"ok"}`)

	steps, err := Session([]domain.Exchange{a, b})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if steps[0].ID == steps[1].ID {
		t.Errorf("expected disambiguated ids for two identical endpoints, got %q and %q", steps[0].ID, steps[1].ID)
	}
}

func TestSession_DropsSensitiveRequestHeaders(t *testing.T) {
	ex := jsonExchange("GET", "http://x/api/users", 200, "", `{"data":[]}`)
	ex.Request.Headers.Set("Authorization", "Bearer secret-token")
	steps, err := Session([]domain.Exchange{ex})
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	if _, ok := steps[0].Test.Request.Headers["Authorization"]; ok {
		t.Error("expected Authorization header to be dropped, same as generate.BuildTestCase")
	}
}

func TestSession_ProducesUniqueIDsAcrossManySteps(t *testing.T) {
	exchanges := []domain.Exchange{
		jsonExchange("POST", "http://x/api/users", 201, `{}`, `{"data":{"id":1}}`),
		jsonExchange("GET", "http://x/api/users/1", 200, "", `{"data":{"id":1}}`),
		jsonExchange("DELETE", "http://x/api/users/1", 204, "", ``),
	}
	steps, err := Session(exchanges)
	if err != nil {
		t.Fatalf("Session: %v", err)
	}
	seen := map[string]bool{}
	for _, s := range steps {
		if seen[s.ID] {
			t.Errorf("duplicate step id %q", s.ID)
		}
		seen[s.ID] = true
	}
}
