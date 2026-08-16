package testdef

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/sandeepv/apilens/internal/domain"
	"gopkg.in/yaml.v3"
)

// DefaultTimeout mirrors testing.timeout's default (10s) when a test does
// not set request.timeout (docs/06-test-dsl.md section 9).
const DefaultTimeout = 10 * time.Second

// DefaultRetries mirrors testing.retries' default (1).
const DefaultRetries = 1

// MaxSupportedVersion is the highest DSL version this build understands.
// A file declaring a higher version is a config error rather than being
// silently mis-parsed under the wrong rules.
const MaxSupportedVersion = 2

// responsesRefPattern detects a "{{responses...." placeholder anywhere in
// a string — used to gate chaining syntax to version: 2 documents
// (docs/06-test-dsl.md section 11) without needing a full DSL v2 grammar
// just to answer "does this test reference another test's response?".
var responsesRefPattern = regexp.MustCompile(`\{\{\s*responses\.`)

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
	if doc.Request.Body.JSON != nil && doc.Request.Body.Raw != "" {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: request.body cannot set both \"json\" and \"raw\"", file), nil)
	}
	if doc.Request.GraphQL != nil && (doc.Request.Body.JSON != nil || doc.Request.Body.Raw != "") {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: request.graphql cannot be combined with request.body", file), nil)
	}
	if doc.Request.GraphQL != nil && strings.TrimSpace(doc.Request.GraphQL.Query) == "" {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: request.graphql.query is required", file), nil)
	}

	method := strings.TrimSpace(doc.Request.Method)
	url := strings.TrimSpace(doc.Request.URL)
	if doc.Request.GraphQL != nil {
		if method == "" {
			method = "POST"
		}
		if url == "" {
			url = "{{base_url}}/graphql"
		}
	}
	if method == "" {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("%s: missing required field \"request.method\"", file), nil)
	}
	if url == "" {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("%s: missing required field \"request.url\"", file), nil)
	}

	// Version dispatch (docs/06-test-dsl.md section 1: "files without it
	// are treated as v1 during MVP"). A version above what this build
	// understands is a config error, not a best-effort parse.
	version := doc.Version
	if version == 0 {
		version = 1
	}
	if version < 1 || version > MaxSupportedVersion {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: unsupported \"version\" %d (this build supports 1-%d)", file, doc.Version, MaxSupportedVersion), nil)
	}

	usesChaining := documentUsesChaining(doc)
	if usesChaining && version < 2 {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: uses {{responses....}} chaining syntax but is not \"version: 2\" — "+
				"chaining is a DSL v2 feature (docs/06-test-dsl.md section 11)", file), nil)
	}
	if doc.ID != "" && version < 2 {
		return domain.TestCase{}, domain.NewConfigError(
			fmt.Sprintf("%s: \"id\" is a DSL v2 field — add \"version: 2\" to use it", file), nil)
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
	cleanupSpec, err := compileCleanup(doc.Cleanup, file)
	if err != nil {
		return domain.TestCase{}, err
	}

	tc := domain.TestCase{
		Name:         doc.Name,
		File:         file,
		Description:  doc.Description,
		Tags:         doc.Tags,
		Skip:         doc.Skip,
		Version:      version,
		ID:           doc.ID,
		UsesChaining: usesChaining,
		Request: domain.RequestTemplate{
			Method:  domain.NormalizeMethod(method),
			URL:     url,
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
		Cleanup: cleanupSpec,
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
	if doc.Request.GraphQL != nil {
		tc.Request.GraphQL = &domain.GraphQLTemplate{
			Query:         doc.Request.GraphQL.Query,
			Variables:     doc.Request.GraphQL.Variables,
			OperationName: doc.Request.GraphQL.OperationName,
		}
	}
	return tc, nil
}

// documentUsesChaining reports whether any interpolatable string field in
// doc contains a "{{responses...." placeholder. Checked across every
// place environment.Resolver.Interpolate is eventually called for this
// test: URL, headers, query, body (raw or JSON string leaves), and auth
// fields — a chaining reference tucked into a header value must be caught
// just as reliably as one in the URL.
func documentUsesChaining(doc document) bool {
	if responsesRefPattern.MatchString(doc.Request.URL) {
		return true
	}
	for _, v := range doc.Request.Headers {
		if responsesRefPattern.MatchString(v) {
			return true
		}
	}
	for _, v := range doc.Request.Query {
		if responsesRefPattern.MatchString(v) {
			return true
		}
	}
	if responsesRefPattern.MatchString(doc.Request.Body.Raw) {
		return true
	}
	if valueUsesChaining(doc.Request.Body.JSON) {
		return true
	}
	if a := doc.Request.Auth; a != nil {
		for _, v := range []string{a.Token, a.Username, a.Password, a.Value} {
			if responsesRefPattern.MatchString(v) {
				return true
			}
		}
	}
	if g := doc.Request.GraphQL; g != nil {
		if responsesRefPattern.MatchString(g.Query) || responsesRefPattern.MatchString(g.OperationName) {
			return true
		}
		if valueUsesChaining(g.Variables) {
			return true
		}
	}
	return false
}

// valueUsesChaining recurses into a decoded JSON body's string leaves,
// mirroring environment.Resolver.interpolateJSONValue's own traversal
// shape so detection stays consistent with what will actually be
// interpolated at run time.
func valueUsesChaining(v any) bool {
	switch t := v.(type) {
	case string:
		return responsesRefPattern.MatchString(t)
	case map[string]any:
		for _, val := range t {
			if valueUsesChaining(val) {
				return true
			}
		}
	case []any:
		for _, val := range t {
			if valueUsesChaining(val) {
				return true
			}
		}
	}
	return false
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
			js := domain.JSONSpec{
				Exists:   j.Exists,
				Equals:   j.Equals,
				Contains: j.Contains,
				Schema:   j.Schema,
				Matches:  j.Matches,
				Length:   j.Length,
			}
			if j.Schema != nil && j.SchemaFile != "" {
				return domain.AssertionSpec{}, domain.NewConfigError(
					fmt.Sprintf("%s: json.%s cannot set both \"schema\" and \"schema_file\"", file, path), nil)
			}
			if j.SchemaFile != "" {
				loaded, err := loadSchemaFile(file, j.SchemaFile)
				if err != nil {
					return domain.AssertionSpec{}, domain.NewConfigError(
						fmt.Sprintf("%s: json.%s.schema_file", file, path), err)
				}
				js.Schema = loaded
			}
			spec.JSON[path] = js
		}
	}
	if a.Duration != nil {
		spec.Duration = &domain.DurationSpec{LessThan: a.Duration.LessThan}
	}
	if a.GraphQL != nil {
		spec.GraphQL = &domain.GraphQLAssertSpec{
			NoErrors:      a.GraphQL.NoErrors,
			HasData:       a.GraphQL.HasData,
			ErrorContains: a.GraphQL.ErrorContains,
		}
	}
	if len(a.DB) > 0 {
		spec.DB = make(map[string][]domain.DBSpec, len(a.DB))
		for conn, docs := range a.DB {
			if len(docs) == 0 {
				return domain.AssertionSpec{}, domain.NewConfigError(
					fmt.Sprintf("%s: db.%s has no checks", file, conn), nil)
			}
			specs := make([]domain.DBSpec, 0, len(docs))
			for i, d := range docs {
				hasSQL := d.Query != ""
				hasMongo := d.Collection != ""
				if hasSQL && hasMongo {
					return domain.AssertionSpec{}, domain.NewConfigError(
						fmt.Sprintf("%s: db.%s[%d] cannot set both \"query\" (SQL) and \"collection\" (MongoDB)", file, conn, i), nil)
				}
				if !hasSQL && !hasMongo {
					return domain.AssertionSpec{}, domain.NewConfigError(
						fmt.Sprintf("%s: db.%s[%d] requires either \"query\" (SQL) or \"collection\" (MongoDB)", file, conn, i), nil)
				}
				specs = append(specs, domain.DBSpec{
					Query:          d.Query,
					Args:           d.Args,
					Collection:     d.Collection,
					Filter:         d.Filter,
					Field:          d.Field,
					RowCountEquals: d.RowCountEquals,
					Exists:         d.Exists,
					Equals:         d.Equals,
				})
			}
			spec.DB[conn] = specs
		}
	}

	if spec.Status == nil && len(spec.Headers) == 0 && spec.Body == nil &&
		len(spec.JSON) == 0 && spec.Duration == nil && spec.GraphQL == nil && len(spec.DB) == 0 {
		return domain.AssertionSpec{}, domain.NewConfigError(
			fmt.Sprintf("%s: test has no assertions under \"assert\"", file), nil)
	}
	return spec, nil
}

// compileCleanup validates and converts the cleanup: block. Every entry
// must set both collection and a non-empty filter — a missing/empty
// filter is rejected at Compile time (not left for dbassert to catch at
// run time) because an empty MongoDB filter matches every document in
// the collection, and "delete everything" is never what an empty filter
// here could have meant.
func compileCleanup(c cleanupDoc, file string) (domain.CleanupSpec, error) {
	spec := domain.CleanupSpec{}
	if len(c.DB) == 0 {
		return spec, nil
	}
	spec.DB = make(map[string][]domain.CleanupDBSpec, len(c.DB))
	for conn, items := range c.DB {
		if len(items) == 0 {
			return domain.CleanupSpec{}, domain.NewConfigError(
				fmt.Sprintf("%s: cleanup.db.%s has no targets", file, conn), nil)
		}
		targets := make([]domain.CleanupDBSpec, 0, len(items))
		for i, item := range items {
			if item.Collection == "" {
				return domain.CleanupSpec{}, domain.NewConfigError(
					fmt.Sprintf("%s: cleanup.db.%s[%d] requires \"collection\"", file, conn, i), nil)
			}
			if isEmptyFilter(item.Filter) {
				return domain.CleanupSpec{}, domain.NewConfigError(
					fmt.Sprintf("%s: cleanup.db.%s[%d] requires a non-empty \"filter\" — an empty filter would delete every document in %q", file, conn, i, item.Collection), nil)
			}
			targets = append(targets, domain.CleanupDBSpec{Collection: item.Collection, Filter: item.Filter})
		}
		spec.DB[conn] = targets
	}
	return spec, nil
}

// isEmptyFilter reports whether filter is nil, or a map/slice with no
// elements — every one of these would compile to a MongoDB filter that
// matches (and, for cleanup, deletes) the entire collection.
func isEmptyFilter(filter any) bool {
	switch f := filter.(type) {
	case nil:
		return true
	case map[string]any:
		return len(f) == 0
	case []any:
		return len(f) == 0
	default:
		return false
	}
}

// loadSchemaFile resolves schemaFile relative to the test file's own
// directory (so a shared schema can live next to the tests that use it,
// e.g. .apilens/tests/users/user.schema.json) and parses it as JSON.
func loadSchemaFile(testFile, schemaFile string) (any, error) {
	path := schemaFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(testFile), schemaFile)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading schema file %s: %w", path, err)
	}
	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("parsing schema file %s: %w", path, err)
	}
	return parsed, nil
}
