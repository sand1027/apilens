package assertions

import (
	"net/http"
	"testing"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
)

func exchangeWithJSON(status int, body string) domain.Exchange {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	return domain.Exchange{
		Response: domain.HTTPResponse{StatusCode: status, Headers: h, Body: []byte(body)},
		Timing:   domain.Timing{Duration: 50 * time.Millisecond},
	}
}

func mustCompile(t *testing.T, spec domain.AssertionSpec) domain.AssertionSet {
	t.Helper()
	e := New()
	set, err := e.Compile(spec)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	return set
}

func TestStatusEquals(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`)
	want := 200
	set := mustCompile(t, domain.AssertionSpec{Status: &domain.StatusSpec{Equals: &want}})
	results := New().Eval(set, ex)
	if len(results) != 1 || !results[0].Passed {
		t.Fatalf("expected passing status.equals, got %+v", results)
	}
}

func TestStatusEquals_Fails(t *testing.T) {
	ex := exchangeWithJSON(500, `{}`)
	want := 200
	set := mustCompile(t, domain.AssertionSpec{Status: &domain.StatusSpec{Equals: &want}})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Fatalf("expected failing status.equals, got pass")
	}
	if results[0].Expected != "200" || results[0].Actual != "500" {
		t.Errorf("unexpected expected/actual: %+v", results[0])
	}
}

func TestHeaderExistsAndContains(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`)
	trueVal := true
	contains := "application/json"
	set := mustCompile(t, domain.AssertionSpec{
		Headers: map[string]domain.HeaderSpec{
			"Content-Type": {Exists: &trueVal, Contains: &contains},
		},
	})
	results := New().Eval(set, ex)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("expected header check to pass: %+v", r)
		}
	}
}

func TestHeaderExists_MissingHeaderFails(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`)
	trueVal := true
	set := mustCompile(t, domain.AssertionSpec{
		Headers: map[string]domain.HeaderSpec{"X-Missing": {Exists: &trueVal}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected header.exists to fail for missing header")
	}
}

func TestJSONExists_DottedPath(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"items":[{"id":1,"name":"ada"}]}}`)
	trueVal := true
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data.items.0.id": {Exists: &trueVal}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.exists to pass for data.items.0.id, got %+v", results[0])
	}
}

func TestJSONEquals(t *testing.T) {
	ex := exchangeWithJSON(200, `{"data":{"id":1}}`)
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data.id": {Equals: float64(1)}},
	})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected json.equals to pass, got %+v", results[0])
	}
}

func TestJSONContains_StringAndArray(t *testing.T) {
	ex := exchangeWithJSON(200, `{"email":"ada@example.com","tags":["a","b"]}`)
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{
			"email": {Contains: "@"},
			"tags":  {Contains: "b"},
		},
	})
	results := New().Eval(set, ex)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("expected json.contains to pass: %+v", r)
		}
	}
}

func TestJSON_NonJSONBodyFailsWithReason(t *testing.T) {
	ex := domain.Exchange{Response: domain.HTTPResponse{StatusCode: 200, Body: []byte("not json")}}
	trueVal := true
	set := mustCompile(t, domain.AssertionSpec{
		JSON: map[string]domain.JSONSpec{"data": {Exists: &trueVal}},
	})
	results := New().Eval(set, ex)
	if results[0].Passed {
		t.Error("expected json.exists on non-JSON body to fail")
	}
	if results[0].Reason != "response is not JSON" {
		t.Errorf("Reason = %q, want %q", results[0].Reason, "response is not JSON")
	}
}

func TestBodyContainsAndNotContains(t *testing.T) {
	ex := exchangeWithJSON(200, `{"id":1}`)
	contains := "id"
	notContains := "error"
	set := mustCompile(t, domain.AssertionSpec{
		Body: &domain.BodySpec{Contains: &contains, NotContains: &notContains},
	})
	results := New().Eval(set, ex)
	for _, r := range results {
		if !r.Passed {
			t.Errorf("expected body assertion to pass: %+v", r)
		}
	}
}

func TestDurationLessThan(t *testing.T) {
	ex := exchangeWithJSON(200, `{}`) // 50ms duration
	want := 1000
	set := mustCompile(t, domain.AssertionSpec{Duration: &domain.DurationSpec{LessThan: &want}})
	results := New().Eval(set, ex)
	if !results[0].Passed {
		t.Errorf("expected duration.less_than to pass: %+v", results[0])
	}
}

func TestCompile_EmptySpecReturnsError(t *testing.T) {
	e := New()
	_, err := e.Compile(domain.AssertionSpec{})
	if err == nil {
		t.Fatal("expected error compiling an empty AssertionSpec")
	}
}
