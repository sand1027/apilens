package domain

import (
	"encoding/json"
	"time"
)

// MarshalJSON keeps the dashboard JSON shape (PascalCase, Body as base64)
// without encoding the error interface, which encoding/json cannot marshal
// when set. A transport failure is sent as Err string | null.
func (e Exchange) MarshalJSON() ([]byte, error) {
	var errMsg *string
	if e.Err != nil {
		s := e.Err.Error()
		errMsg = &s
	}
	return json.Marshal(struct {
		ID        ExchangeID   `json:"ID"`
		Display   DisplayID    `json:"Display"`
		Request   HTTPRequest  `json:"Request"`
		Response  HTTPResponse `json:"Response"`
		Timing    Timing       `json:"Timing"`
		Timestamp time.Time    `json:"Timestamp"`
		Redacted  bool         `json:"Redacted"`
		Err       *string      `json:"Err"`
	}{
		ID:        e.ID,
		Display:   e.Display,
		Request:   e.Request,
		Response:  e.Response,
		Timing:    e.Timing,
		Timestamp: e.Timestamp,
		Redacted:  e.Redacted,
		Err:       errMsg,
	})
}
