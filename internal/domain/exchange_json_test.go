package domain

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestExchangeMarshalJSON_WithTransportError(t *testing.T) {
	ex := Exchange{
		Display: 7,
		Request: HTTPRequest{Method: "POST", URL: "http://localhost:3000/graphql", Body: []byte(`{"query":"{ ping }"}`)},
		Response: HTTPResponse{StatusCode: 200, Body: []byte(`{"data":{"ping":true}}`)},
		Err:     errors.New("dial tcp: connection refused"),
	}
	b, err := json.Marshal(ex)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["Err"] != "dial tcp: connection refused" {
		t.Errorf("Err = %#v", got["Err"])
	}
	if got["Display"] != float64(7) {
		t.Errorf("Display = %#v", got["Display"])
	}
}

func TestExchangeMarshalJSON_NilErrorIsNull(t *testing.T) {
	b, err := json.Marshal(Exchange{Display: 1, Request: HTTPRequest{Method: "GET", URL: "http://localhost:3000/x"}})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got["Err"] != nil {
		t.Errorf("Err = %#v, want null", got["Err"])
	}
}
