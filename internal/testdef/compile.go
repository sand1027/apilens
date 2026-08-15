package testdef

import (
	"fmt"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// DefaultTimeout mirrors testing.timeout's default (10s) when a test does
// not set request.timeout (docs/06-test-dsl.md section 9).
const DefaultTimeout = 10 * time.Second

// DefaultRetries mirrors testing.retries' default (1).
const DefaultRetries = 1

// Compile parses raw YAML bytes and compiles it into a domain.TestCase.
// `file` is used only for error messages and TestCase.File.
func Compile(raw []byte, file string) (domain.TestCase, error) {
	var doc document
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("parsing %s", file), err)
	}
	return compileDocument(doc, file)
}

func compileDocument(doc document, file string) (domain.TestCase, error) {
	if doc.Name == "" {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("%s: missing required field \"name\"", file), nil)
	}
	if doc.Request.Method == "" {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("%s: missing required field \"request.method\"", file), nil)
	}
	if doc.Request.URL == "" {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("%s: missing required field \"request.url\"", file), nil)
	}
	if doc.Request.Body.JSON != nil && doc.Request.Body.Raw != "" {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: request.body cannot set both \"json\" and \"raw\"", file), nil)
	}

	timeout := DefaultTimeout
	if doc.Request.Timeout != "" {
		d, err := time.ParseDuration(doc.Request.Timeout)
		if err != nil {
			return domain.TestCase{}, domain.NewConfigError(
				fmt.Sprintf("%s: invalid request.timeout %q", file, doc.Request.Timeout), err)
		}
		timeout = d
	}

	retries := DefaultRetries
	if doc.Retries != nil {
		retries = *doc.Retries
	}

	assertSpec, err := compileAssert(doc.Assert, file)
	if err != nil {
		return domain.TestCase{}, err
	}

	tc := domain.TestCase{
		Name:        doc.Name,
		File:        file,
		Description: doc.Description,
		Tags:        doc.Tags,
		Skip:        doc.Skip,
		Request: domain.RequestTemplate{
			Method:  domain.NormalizeMethod(doc.Request.Method),
			URL:     doc.Request.URL,
			Headers: doc.Request.Headers,
			Query:   doc.Request.Query,
			Body: domain.BodyTemplate{
				JSON:        doc.Request.Body.JSON,
				Raw:         doc.Request.Body.Raw,
				ContentType: doc.Request.Body.ContentType,
			},
			Timeout: timeout,
		},
		Assert:  assertSpec,
		Timeout: timeout,
		Retries: retries,
	}
	if doc.Request.Auth != nil {
		tc.Request.Auth = &domain.AuthTemplate{
			Type:     doc.Request.Auth.Type,
			Token:    doc.Request.Auth.Token,
			Username: doc.Request.Auth.Username,
			Password: doc.Request.Auth.Password,
			Header:   doc.Request.Auth.Header,
			Value:    doc.Request.Auth.Value,
			Name:     doc.Request.Auth.Name,
		}
	}
	return tc, nil
}

func compileAssert(a assertDoc, file string) (domain.AssertionSpec, error) {
	spec := domain.AssertionSpec{}

	if a.Status != nil {
		spec.Status = &domain.StatusSpec{Equals: a.Status.Equals, NotEquals: a.Status.NotEquals}
	}
	if len(a.Headers) > 0 {
		spec.Headers = make(map[string]domain.HeaderSpec, len(a.Headers))
		for name, h := range a.Headers {
			spec.Headers[name] = domain.HeaderSpec{Exists: h.Exists, Equals: h.Equals, Contains: h.Contains}
		}
	}
	if a.Body != nil {
		spec.Body = &domain.BodySpec{Contains: a.Body.Contains, NotContains: a.Body.NotContains}
	}
	if len(a.JSON) > 0 {
		spec.JSON = make(map[string]domain.JSONSpec, len(a.JSON))
		for path, j := range a.JSON {
			spec.JSON[path] = domain.JSONSpec{Exists: j.Exists, Equals: j.Equals, Contains: j.Contains}
		}
	}
	if a.Duration != nil {
		spec.Duration = &domain.DurationSpec{LessThan: a.Duration.LessThan}
	}

	if spec.Status == nil && len(spec.Headers) == 0 && spec.Body == nil &&
		len(spec.JSON) == 0 && spec.Duration == nil {
		return domain.AssertionSpec{}, domain.NewConfigError(
			fmt.Sprintf("%s: test has no assertions under \"assert\"", file), nil)
	}
	return spec, nil
}
