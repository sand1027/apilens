package domain

import (
	"net/http"
	"net/url"
	"time"
)

// HTTPRequest is a fully-interpolated, ready-to-send request. Nothing in
// this struct still contains "{{var}}" or "${ENV}" placeholders — that
// happens upstream in internal/environment.
type HTTPRequest struct {
	Method  Method
	URL     string
	Headers http.Header
	Query   url.Values
	Body    []byte
	Timeout time.Duration
}

// Timing captures start/end/duration for a single HTTP exchange.
type Timing struct {
	Start    time.Time
	End      time.Time
	Duration time.Duration
}

// HTTPResponse is the observed result of executing an HTTPRequest.
type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Truncated  bool
}

// Exchange is one HTTP request/response pair as observed by the runner or
// proxy, plus metadata used by history, replay, and reporters.
type Exchange struct {
	ID        ExchangeID
	Display   DisplayID
	Request   HTTPRequest
	Response  HTTPResponse
	Timing    Timing
	Timestamp time.Time
	Redacted  bool
	Err       error // set when the transport call itself failed
}
