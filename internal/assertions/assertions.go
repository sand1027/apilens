// Package assertions compiles domain.AssertionSpec into an executable
// domain.AssertionSet and evaluates it against a domain.Exchange. Bad DSL
// is caught at Compile time before any HTTP call; Eval never panics on
// unexpected JSON (docs/04-interfaces.md section 7).
package assertions

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/sandeepv/apilens/internal/dbassert"
	"github.com/sandeepv/apilens/internal/domain"
)

// Engine implements docs/04-interfaces.md section 7's Engine interface.
// dbRegistry is nil unless a caller opts in via WithDBRegistry — most
// Engine instances never touch a database at all (plan.md v9's db.*
// assertions are opt-in, not core).
type Engine struct {
	dbRegistry *dbassert.Registry
}

// Option configures New.
type Option func(*Engine)

// WithDBRegistry enables db.* assertions (plan.md v9: "Database
// assertions (opt-in plugin)") by giving the Engine a configured
// dbassert.Registry to query against. Without this option, any db.*
// assertion in a test file is a config error at Compile time — the
// engine has no connections to query and says so explicitly rather than
// silently skipping the check.
func WithDBRegistry(reg *dbassert.Registry) Option {
	return func(e *Engine) { e.dbRegistry = reg }
}

// New builds an assertions Engine. Built-in kinds (status/header/body/
// json/duration) need no state and are always available; db.* checks
// need an opted-in dbassert.Registry (docs/03-plugins.md section 10 lists
// reasons a feature stays core vs. plugin — connection lifecycle and an
// external credential requirement are exactly the kind of thing that
// stays optional). The Evaluator/Check port below still exists for
// future plugin kinds, per docs/03-plugins.md section 6.
func New(opts ...Option) *Engine {
	e := &Engine{}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Compile turns a validated AssertionSpec into an executable AssertionSet.
func (e *Engine) Compile(spec domain.AssertionSpec) (domain.AssertionSet, error) {
	var checks []domain.Check

	if spec.Status != nil {
		if spec.Status.Equals != nil {
			checks = append(checks, statusEqualsCheck{want: *spec.Status.Equals})
		}
		if spec.Status.NotEquals != nil {
			checks = append(checks, statusNotEqualsCheck{want: *spec.Status.NotEquals})
		}
	}

	for name, h := range spec.Headers {
		if h.Exists != nil {
			checks = append(checks, headerExistsCheck{name: name, want: *h.Exists})
		}
		if h.Equals != nil {
			checks = append(checks, headerEqualsCheck{name: name, want: *h.Equals})
		}
		if h.Contains != nil {
			checks = append(checks, headerContainsCheck{name: name, want: *h.Contains})
		}
	}

	if spec.Body != nil {
		if spec.Body.Contains != nil {
			checks = append(checks, bodyContainsCheck{want: *spec.Body.Contains})
		}
		if spec.Body.NotContains != nil {
			checks = append(checks, bodyNotContainsCheck{want: *spec.Body.NotContains})
		}
	}

	for path, j := range spec.JSON {
		if j.Exists != nil {
			checks = append(checks, jsonExistsCheck{path: path, want: *j.Exists})
		}
		if j.Equals != nil {
			checks = append(checks, jsonEqualsCheck{path: path, want: j.Equals})
		}
		if j.Contains != nil {
			checks = append(checks, jsonContainsCheck{path: path, want: j.Contains})
		}
		if j.Schema != nil {
			check, err := newJSONSchemaCheck(path, j.Schema)
			if err != nil {
				return domain.AssertionSet{}, domain.NewConfigError(
					fmt.Sprintf("compiling json.%s.schema", path), err)
			}
			checks = append(checks, check)
		}
		if j.Matches != nil {
			check, err := newJSONMatchesCheck(path, *j.Matches)
			if err != nil {
				return domain.AssertionSet{}, domain.NewConfigError(
					fmt.Sprintf("compiling json.%s.matches", path), err)
			}
			checks = append(checks, check)
		}
		if j.Length != nil {
			checks = append(checks, jsonLengthCheck{path: path, want: *j.Length})
		}
	}

	if spec.Duration != nil && spec.Duration.LessThan != nil {
		checks = append(checks, durationLessThanCheck{wantMS: *spec.Duration.LessThan})
	}

	if spec.GraphQL != nil {
		if spec.GraphQL.NoErrors != nil {
			checks = append(checks, graphqlNoErrorsCheck{wantNone: *spec.GraphQL.NoErrors})
		}
		if spec.GraphQL.HasData != nil {
			checks = append(checks, graphqlHasDataCheck{want: *spec.GraphQL.HasData})
		}
		if spec.GraphQL.ErrorContains != nil {
			checks = append(checks, graphqlErrorContainsCheck{want: *spec.GraphQL.ErrorContains})
		}
	}

	for connName, d := range spec.DB {
		if err := dbassert.ValidateReadOnly(d.Query); err != nil {
			return domain.AssertionSet{}, domain.NewConfigError(
				fmt.Sprintf("compiling db.%s", connName), err)
		}
		if e.dbRegistry == nil {
			return domain.AssertionSet{}, domain.NewConfigError(
				fmt.Sprintf("db.%s used but no db connections are configured — add db.connections.%s to config.yaml", connName, connName), nil)
		}
		check, err := newDBCheck(connName, d, e.dbRegistry)
		if err != nil {
			return domain.AssertionSet{}, domain.NewConfigError(
				fmt.Sprintf("compiling db.%s", connName), err)
		}
		checks = append(checks, check)
	}

	if len(checks) == 0 {
		return domain.AssertionSet{}, domain.NewConfigError("assertion spec compiled to zero checks", nil)
	}
	return domain.AssertionSet{Checks: checks}, nil
}

// Eval runs every compiled check against ex and returns their results in
// declaration order.
func (e *Engine) Eval(set domain.AssertionSet, ex domain.Exchange) []domain.AssertionResult {
	results := make([]domain.AssertionResult, 0, len(set.Checks))
	for _, c := range set.Checks {
		results = append(results, c.Eval(ex))
	}
	return results
}

// --- status ---

type statusEqualsCheck struct{ want int }

func (c statusEqualsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	passed := ex.Response.StatusCode == c.want
	return domain.AssertionResult{
		Kind:     domain.KindStatusEquals,
		Passed:   passed,
		Expected: strconv.Itoa(c.want),
		Actual:   strconv.Itoa(ex.Response.StatusCode),
	}
}

type statusNotEqualsCheck struct{ want int }

func (c statusNotEqualsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	passed := ex.Response.StatusCode != c.want
	return domain.AssertionResult{
		Kind:     domain.KindStatusNotEquals,
		Passed:   passed,
		Expected: "not " + strconv.Itoa(c.want),
		Actual:   strconv.Itoa(ex.Response.StatusCode),
	}
}

// --- headers ---
// Header lookups are case-insensitive via http.Header.Get, which
// canonicalizes MIME case (docs/06-test-dsl.md section 5).

type headerExistsCheck struct {
	name string
	want bool
}

func (c headerExistsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	_, present := lookupHeader(ex.Response.Headers, c.name)
	passed := present == c.want
	return domain.AssertionResult{
		Kind:     domain.KindHeaderExists,
		Target:   c.name,
		Passed:   passed,
		Expected: strconv.FormatBool(c.want),
		Actual:   strconv.FormatBool(present),
	}
}

type headerEqualsCheck struct {
	name string
	want string
}

func (c headerEqualsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, present := lookupHeader(ex.Response.Headers, c.name)
	passed := present && val == c.want
	return domain.AssertionResult{
		Kind:     domain.KindHeaderEquals,
		Target:   c.name,
		Passed:   passed,
		Expected: c.want,
		Actual:   val,
		Reason:   missingReason(present, c.name),
	}
}

type headerContainsCheck struct {
	name string
	want string
}

func (c headerContainsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, present := lookupHeader(ex.Response.Headers, c.name)
	passed := present && strings.Contains(val, c.want)
	return domain.AssertionResult{
		Kind:     domain.KindHeaderContains,
		Target:   c.name,
		Passed:   passed,
		Expected: c.want,
		Actual:   val,
		Reason:   missingReason(present, c.name),
	}
}

func lookupHeader(h http.Header, name string) (string, bool) {
	if h == nil {
		return "", false
	}
	values, ok := h[http.CanonicalHeaderKey(name)]
	if !ok || len(values) == 0 {
		return "", false
	}
	return values[0], true
}

func missingReason(present bool, name string) string {
	if present {
		return ""
	}
	return fmt.Sprintf("header %q not present", name)
}

// --- body (raw substring) ---

type bodyContainsCheck struct{ want string }

func (c bodyContainsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	passed := strings.Contains(string(ex.Response.Body), c.want)
	return domain.AssertionResult{Kind: domain.KindBodyContains, Passed: passed, Expected: c.want}
}

type bodyNotContainsCheck struct{ want string }

func (c bodyNotContainsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	passed := !strings.Contains(string(ex.Response.Body), c.want)
	return domain.AssertionResult{Kind: domain.KindBodyNotContains, Passed: passed, Expected: "not " + c.want}
}

// --- json (dotted paths) ---

type jsonExistsCheck struct {
	path string
	want bool
}

func (c jsonExistsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	_, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONExists, c.path, err)
	}
	passed := found == c.want
	return domain.AssertionResult{
		Kind:     domain.KindJSONExists,
		Target:   c.path,
		Passed:   passed,
		Expected: strconv.FormatBool(c.want),
		Actual:   strconv.FormatBool(found),
	}
}

type jsonEqualsCheck struct {
	path string
	want any
}

func (c jsonEqualsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONEquals, c.path, err)
	}
	passed := found && deepEqual(val, c.want)
	return domain.AssertionResult{
		Kind:     domain.KindJSONEquals,
		Target:   c.path,
		Passed:   passed,
		Expected: fmt.Sprintf("%v", c.want),
		Actual:   fmt.Sprintf("%v", val),
		Reason:   missingReason(found, c.path),
	}
}

type jsonContainsCheck struct {
	path string
	want any
}

func (c jsonContainsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	val, found, err := lookupJSONPath(ex.Response.Body, c.path)
	if err != nil {
		return jsonErrorResult(domain.KindJSONContains, c.path, err)
	}
	if !found {
		return domain.AssertionResult{
			Kind: domain.KindJSONContains, Target: c.path, Passed: false,
			Expected: fmt.Sprintf("%v", c.want), Reason: missingReason(false, c.path),
		}
	}
	passed := jsonContains(val, c.want)
	return domain.AssertionResult{
		Kind:     domain.KindJSONContains,
		Target:   c.path,
		Passed:   passed,
		Expected: fmt.Sprintf("%v", c.want),
		Actual:   fmt.Sprintf("%v", val),
	}
}

func jsonErrorResult(kind domain.AssertionKind, path string, err error) domain.AssertionResult {
	return domain.AssertionResult{Kind: kind, Target: path, Passed: false, Reason: err.Error()}
}

func jsonContains(val, want any) bool {
	switch v := val.(type) {
	case string:
		s, ok := want.(string)
		return ok && strings.Contains(v, s)
	case []any:
		for _, item := range v {
			if deepEqual(item, want) {
				return true
			}
		}
		return false
	default:
		return false
	}
}

func deepEqual(a, b any) bool {
	ab, err1 := json.Marshal(a)
	bb, err2 := json.Marshal(b)
	if err1 != nil || err2 != nil {
		return false
	}
	return string(ab) == string(bb)
}

// lookupJSONPath resolves a dotted path (docs/06-test-dsl.md section 6)
// against a JSON response body. Returns (nil, false, err) if the body is
// not JSON — every json.* assertion fails with "response is not JSON" per
// spec.
func lookupJSONPath(body []byte, path string) (any, bool, error) {
	if len(body) == 0 {
		return nil, false, fmt.Errorf("response is not JSON")
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false, fmt.Errorf("response is not JSON")
	}
	if path == "" {
		return parsed, true, nil
	}
	segments := strings.Split(path, ".")
	cur := parsed
	for _, seg := range segments {
		switch node := cur.(type) {
		case map[string]any:
			v, ok := node[seg]
			if !ok {
				return nil, false, nil
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(seg)
			if err != nil || idx < 0 || idx >= len(node) {
				return nil, false, nil
			}
			cur = node[idx]
		default:
			return nil, false, nil
		}
	}
	return cur, true, nil
}

// --- duration ---

type durationLessThanCheck struct{ wantMS int }

func (c durationLessThanCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	actualMS := ex.Timing.Duration.Milliseconds()
	passed := actualMS < int64(c.wantMS)
	return domain.AssertionResult{
		Kind:     domain.KindDurationLess,
		Passed:   passed,
		Expected: strconv.Itoa(c.wantMS),
		Actual:   strconv.FormatInt(actualMS, 10),
	}
}

// --- graphql envelope ---

type graphqlEnvelope struct {
	Data   any   `json:"data"`
	Errors []any `json:"errors"`
}

func parseGraphQLEnvelope(body []byte) (graphqlEnvelope, error) {
	var env graphqlEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return graphqlEnvelope{}, fmt.Errorf("response is not JSON")
	}
	return env, nil
}

func errorMessages(errors []any) []string {
	var msgs []string
	for _, e := range errors {
		switch t := e.(type) {
		case map[string]any:
			if m, ok := t["message"].(string); ok {
				msgs = append(msgs, m)
			} else {
				msgs = append(msgs, fmt.Sprintf("%v", e))
			}
		default:
			msgs = append(msgs, fmt.Sprintf("%v", e))
		}
	}
	return msgs
}

type graphqlNoErrorsCheck struct{ wantNone bool }

func (c graphqlNoErrorsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	env, err := parseGraphQLEnvelope(ex.Response.Body)
	if err != nil {
		return domain.AssertionResult{Kind: domain.KindGraphQLNoErrors, Passed: false, Reason: err.Error()}
	}
	hasErrors := len(env.Errors) > 0
	passed := hasErrors != c.wantNone
	actual := "no errors"
	if hasErrors {
		actual = strings.Join(errorMessages(env.Errors), "; ")
	}
	expected := "no GraphQL errors"
	if !c.wantNone {
		expected = "GraphQL errors present"
	}
	return domain.AssertionResult{
		Kind:     domain.KindGraphQLNoErrors,
		Passed:   passed,
		Expected: expected,
		Actual:   actual,
	}
}

type graphqlHasDataCheck struct{ want bool }

func (c graphqlHasDataCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	env, err := parseGraphQLEnvelope(ex.Response.Body)
	if err != nil {
		return domain.AssertionResult{Kind: domain.KindGraphQLHasData, Passed: false, Reason: err.Error()}
	}
	hasData := env.Data != nil
	passed := hasData == c.want
	return domain.AssertionResult{
		Kind:     domain.KindGraphQLHasData,
		Passed:   passed,
		Expected: strconv.FormatBool(c.want),
		Actual:   strconv.FormatBool(hasData),
	}
}

type graphqlErrorContainsCheck struct{ want string }

func (c graphqlErrorContainsCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	env, err := parseGraphQLEnvelope(ex.Response.Body)
	if err != nil {
		return domain.AssertionResult{Kind: domain.KindGraphQLErrorContains, Passed: false, Reason: err.Error()}
	}
	msgs := errorMessages(env.Errors)
	joined := strings.Join(msgs, "; ")
	passed := strings.Contains(joined, c.want)
	return domain.AssertionResult{
		Kind:     domain.KindGraphQLErrorContains,
		Passed:   passed,
		Expected: c.want,
		Actual:   joined,
	}
}
