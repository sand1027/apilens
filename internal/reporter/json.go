package reporter

import (
	"encoding/json"
	"io"
	"os"

	"github.com/sandeepv/apilens/internal/domain"
)

// JSONReporter buffers results and writes the whole document once in
// SuiteFinished, so the output is always a single valid JSON value
// (docs/04-interfaces.md section 9). Schema matches
// docs/11-risks-and-gaps.md G25. No raw headers or bodies are included by
// default (docs/09-security.md section 3, ADR-006).
type JSONReporter struct {
	out  io.Writer
	meta domain.SuiteMeta
}

// NewJSON builds a JSONReporter writing to w (os.Stdout in the CLI).
func NewJSON(w io.Writer) *JSONReporter {
	if w == nil {
		w = os.Stdout
	}
	return &JSONReporter{out: w}
}

func (j *JSONReporter) Name() string      { return "json" }
func (j *JSONReporter) Formats() []string { return []string{"json"} }

func (j *JSONReporter) Start(meta domain.SuiteMeta) {
	j.meta = meta
}

func (j *JSONReporter) TestFinished(result domain.TestResult) {
	// JSON reporter buffers and writes once at SuiteFinished so the
	// document stays valid (docs/04-interfaces.md section 9).
}

// jsonAssertion mirrors the "assertions" shape in the documented schema.
type jsonAssertion struct {
	Kind     string `json:"kind"`
	Passed   bool   `json:"passed"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

type jsonResult struct {
	Name       string          `json:"name"`
	File       string          `json:"file"`
	Status     string          `json:"status"`
	Method     string          `json:"method"`
	URL        string          `json:"url"`
	HTTPStatus int             `json:"http_status"`
	DurationMS int64           `json:"duration_ms"`
	Assertions []jsonAssertion `json:"assertions,omitempty"`
	Error      string          `json:"error,omitempty"`
}

type jsonDocument struct {
	Version        int           `json:"version"`
	Env            string        `json:"env"`
	Counts         domain.Counts `json:"counts"`
	SuccessPercent int           `json:"success_percent"`
	DurationMS     int64         `json:"duration_ms"`
	Results        []jsonResult  `json:"results"`
}

func (j *JSONReporter) SuiteFinished(report domain.Report) error {
	results := make([]jsonResult, 0, len(report.Results))
	for _, r := range report.Results {
		assertions := make([]jsonAssertion, 0, len(r.Assertions))
		for _, a := range r.Assertions {
			assertions = append(assertions, jsonAssertion{
				Kind:     string(a.Kind),
				Passed:   a.Passed,
				Expected: a.Expected,
				Actual:   a.Actual,
				Reason:   a.Reason,
			})
		}
		results = append(results, jsonResult{
			Name:       r.Name,
			File:       r.File,
			Status:     string(r.Status),
			Method:     string(r.Method),
			URL:        r.URL,
			HTTPStatus: r.HTTPStatus,
			DurationMS: r.DurationMS,
			Assertions: assertions,
			Error:      r.Error,
		})
	}
	doc := jsonDocument{
		Version:        1,
		Env:            report.Env,
		Counts:         report.Counts,
		SuccessPercent: int(report.Counts.SuccessPercent()),
		DurationMS:     report.DurationMS,
		Results:        results,
	}
	enc := json.NewEncoder(j.out)
	enc.SetIndent("", "  ")
	return enc.Encode(doc)
}
