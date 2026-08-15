package reporter

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

// JUnitReporter writes the standard JUnit XML shape most CI systems
// (GitHub Actions, GitLab, Jenkins) already know how to parse
// (plan.md v5: "JUnit XML reporter"). Like JSONReporter, it buffers and
// writes once in SuiteFinished so the document is always valid XML
// (docs/04-interfaces.md section 9). No raw headers or bodies are
// included — same no-secrets-in-reports policy as the JSON reporter
// (docs/09-security.md section 3, ADR-006).
type JUnitReporter struct {
	out io.Writer
}

// NewJUnit builds a JUnitReporter writing to w (os.Stdout in the CLI).
func NewJUnit(w io.Writer) *JUnitReporter {
	if w == nil {
		w = os.Stdout
	}
	return &JUnitReporter{out: w}
}

func (j *JUnitReporter) Name() string      { return "junit" }
func (j *JUnitReporter) Formats() []string { return []string{"junit"} }

func (j *JUnitReporter) Start(meta domain.SuiteMeta) {}

func (j *JUnitReporter) TestFinished(result domain.TestResult) {
	// Buffered like JSONReporter; written once in SuiteFinished.
}

// junitTestSuites is the root element most CI JUnit parsers expect,
// even for a single suite.
type junitTestSuites struct {
	XMLName xml.Name         `xml:"testsuites"`
	Suites  []junitTestSuite `xml:"testsuite"`
}

type junitTestSuite struct {
	Name      string          `xml:"name,attr"`
	Tests     int             `xml:"tests,attr"`
	Failures  int             `xml:"failures,attr"`
	Errors    int             `xml:"errors,attr"`
	Skipped   int             `xml:"skipped,attr"`
	Time      string          `xml:"time,attr"`
	TestCases []junitTestCase `xml:"testcase"`
}

type junitTestCase struct {
	Name      string        `xml:"name,attr"`
	ClassName string        `xml:"classname,attr"`
	Time      string        `xml:"time,attr"`
	Failure   *junitFailure `xml:"failure,omitempty"`
	Error     *junitFailure `xml:"error,omitempty"`
	Skipped   *junitSkipped `xml:"skipped,omitempty"`
}

type junitFailure struct {
	Message string `xml:"message,attr"`
	Content string `xml:",chardata"`
}

type junitSkipped struct{}

func (j *JUnitReporter) SuiteFinished(report domain.Report) error {
	suite := junitTestSuite{
		Name:     firstNonEmptyStr(report.Env, "apilens"),
		Tests:    report.Counts.Tests,
		Failures: report.Counts.Failed,
		Errors:   report.Counts.Errored,
		Skipped:  report.Counts.Skipped,
		Time:     formatSeconds(report.DurationMS),
	}

	for _, r := range report.Results {
		tc := junitTestCase{
			Name:      r.Name,
			ClassName: classNameFor(r),
			Time:      formatSeconds(r.DurationMS),
		}
		switch r.Status {
		case domain.StatusFailed:
			tc.Failure = &junitFailure{
				Message: failureMessage(r),
				Content: failureDetail(r),
			}
		case domain.StatusErrored:
			tc.Error = &junitFailure{
				Message: r.Error,
				Content: r.Error,
			}
		case domain.StatusSkipped:
			tc.Skipped = &junitSkipped{}
		}
		suite.TestCases = append(suite.TestCases, tc)
	}

	doc := junitTestSuites{Suites: []junitTestSuite{suite}}
	if _, err := io.WriteString(j.out, xml.Header); err != nil {
		return err
	}
	enc := xml.NewEncoder(j.out)
	enc.Indent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return domain.NewConfigError("encoding JUnit XML", err)
	}
	_, err := io.WriteString(j.out, "\n")
	return err
}

// classNameFor gives CI dashboards a sensible grouping — the test file's
// directory under .apilens/tests, falling back to the method.
func classNameFor(r domain.TestResult) string {
	if r.File != "" {
		return r.File
	}
	return string(r.Method)
}

func failureMessage(r domain.TestResult) string {
	for _, a := range r.Assertions {
		if !a.Passed {
			return fmt.Sprintf("%s: expected %s, got %s", a.Kind, a.Expected, a.Actual)
		}
	}
	return "assertion failed"
}

func failureDetail(r domain.TestResult) string {
	var b strings.Builder
	for _, a := range r.Assertions {
		if a.Passed {
			continue
		}
		fmt.Fprintf(&b, "%s: expected %s, got %s", a.Kind, a.Expected, a.Actual)
		if a.Reason != "" {
			fmt.Fprintf(&b, " (%s)", a.Reason)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func formatSeconds(ms int64) string {
	return fmt.Sprintf("%.3f", float64(ms)/1000)
}

func firstNonEmptyStr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
