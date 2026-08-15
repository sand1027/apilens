// Package recording implements plan.md v9's "Test recording sessions":
// turn an ORDERED sequence of already-captured domain.Exchange values
// (from internal/history, i.e. `apilens watch`) into a chained DSL v2
// suite (internal/testdef's version: 2 / id / {{responses...}} support,
// plan.md v9's other Ships item). A recording session is the natural
// on-ramp to chaining — most real chained flows (login then an
// authenticated call) start life as a manually-clicked-through sequence
// captured by watch, not hand-written YAML.
//
// Correlation between steps is intentionally literal, not semantic: a
// step's request value (a URL path segment, a query value, or a JSON
// body leaf) is rewritten to reference an earlier step's response only
// when it is a byte-for-byte match against a value found somewhere in
// that earlier response's JSON body. This is the same "never invent"
// discipline the rest of ApiLens applies (docs/11-risks-and-gaps.md R2):
// guessing that two DIFFERENT values are "probably related" would produce
// a chained suite that looks right but silently references the wrong
// field the moment real data changes.
package recording

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
)

// Step is one recorded test in the session, already correlated against
// earlier steps.
type Step struct {
	ID   string
	Test domain.TestCase
}

// Session turns exchanges (assumed oldest-first — the same order
// internal/history.Store.List returns) into a chained DSL v2 suite. Each
// exchange becomes exactly one Step, in the same order, so the resulting
// suite replays the recorded session verbatim.
func Session(exchanges []domain.Exchange) ([]Step, error) {
	if len(exchanges) == 0 {
		return nil, domain.NewConfigError("recording session has no captured exchanges to build a suite from", nil)
	}

	steps := make([]Step, 0, len(exchanges))
	usedIDs := make(map[string]bool, len(exchanges))
	// responseValues accumulates every leaf value seen in EARLIER steps'
	// response bodies, each tagged with the reference expression that
	// would reproduce it ("{{responses.<id>.body.<path>}}"). Only earlier
	// steps are ever consulted — a step can never chain off a step that
	// hasn't run yet (docs/06-test-dsl.md section 12's ordering
	// discipline, extended to recording).
	responseValues := map[string]string{}

	for i, ex := range exchanges {
		id := uniqueStepID(ex, i, usedIDs)
		usedIDs[id] = true

		tc := buildChainedTestCase(ex, id, responseValues)
		steps = append(steps, Step{ID: id, Test: tc})

		collectResponseValues(id, ex.Response.Body, responseValues)
	}
	return steps, nil
}

// slugPattern mirrors generate's own slug pattern for filename-safe,
// human-readable step ids.
var slugPattern = regexp.MustCompile(`[^a-zA-Z0-9]+`)

// uniqueStepID builds a readable id like "post-api-users" from the
// exchange's method+path, disambiguating repeats (the same endpoint
// called twice in one session) with a numeric suffix so every id in the
// suite stays unique (required by app.RunSuite's validateChainedSuite).
func uniqueStepID(ex domain.Exchange, index int, used map[string]bool) string {
	method := strings.ToLower(string(ex.Request.Method))
	path := requestPath(ex.Request.URL)
	slug := strings.Trim(slugPattern.ReplaceAllString(path, "-"), "-")
	base := method
	if slug != "" {
		base = method + "-" + slug
	}
	if !used[base] {
		return base
	}
	for n := 2; ; n++ {
		candidate := fmt.Sprintf("%s-%d", base, n)
		if !used[candidate] {
			return candidate
		}
	}
}

// buildChainedTestCase starts from generate's own request-building logic
// (same header-dropping / login-payload-dropping rules — chaining must
// not become a second, laxer path for leaking secrets into a generated
// file) and then rewrites the URL/body wherever a literal match against
// an earlier response is found.
func buildChainedTestCase(ex domain.Exchange, id string, responseValues map[string]string) domain.TestCase {
	tc := generate.BuildTestCase(ex)
	tc.Version = 2
	tc.ID = id
	tc.Tags = append(tc.Tags, "recorded")

	tc.Request.URL = rewriteURLWithChaining(tc.Request.URL, responseValues)
	if tc.Request.Body.JSON != nil {
		rewritten, used := rewriteJSONWithChaining(tc.Request.Body.JSON, responseValues)
		tc.Request.Body.JSON = rewritten
		if used {
			tc.UsesChaining = true
		}
	}
	if strings.Contains(tc.Request.URL, "{{responses.") {
		tc.UsesChaining = true
	}
	return tc
}

// requestPath extracts just the path (drops scheme/host/query), mirroring
// internal/generate's own helper of the same name and purpose.
func requestPath(rawURL string) string {
	if idx := strings.Index(rawURL, "://"); idx >= 0 {
		rest := rawURL[idx+3:]
		if slash := strings.Index(rest, "/"); slash >= 0 {
			rawURL = rest[slash:]
		} else {
			rawURL = "/"
		}
	}
	if q := strings.IndexByte(rawURL, '?'); q >= 0 {
		rawURL = rawURL[:q]
	}
	if rawURL == "" {
		rawURL = "/"
	}
	return rawURL
}

// rewriteURLWithChaining replaces any "/"-delimited path segment whose
// literal text exactly matches a previously recorded response value with
// that value's chaining reference. Only whole segments are considered
// (never a substring within one) — a partial match would produce a
// reference that happens to work today and breaks the moment the real
// value's length changes.
func rewriteURLWithChaining(url string, responseValues map[string]string) string {
	segments := strings.Split(url, "/")
	for i, seg := range segments {
		if ref, ok := responseValues[seg]; ok {
			segments[i] = ref
		}
	}
	return strings.Join(segments, "/")
}

// rewriteJSONWithChaining recurses into a decoded JSON body's string
// leaves, replacing any leaf whose value exactly matches a previously
// recorded response value. Non-string leaves (numbers, bools) are also
// checked via their JSON-encoded form, since a chained numeric id
// (e.g. a user's integer id used as a foreign key in a later request
// body) is a common real case — but the replacement is still always a
// string reference (the interpolated request body always sends the
// value as a string once substituted, same as every other {{var}}
// placeholder in the DSL).
func rewriteJSONWithChaining(v any, responseValues map[string]string) (any, bool) {
	used := false
	switch t := v.(type) {
	case string:
		if ref, ok := responseValues[t]; ok {
			return ref, true
		}
		return t, false
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, val := range t {
			rewritten, u := rewriteJSONWithChaining(val, responseValues)
			out[k] = rewritten
			used = used || u
		}
		return out, used
	case []any:
		out := make([]any, len(t))
		for i, val := range t {
			rewritten, u := rewriteJSONWithChaining(val, responseValues)
			out[i] = rewritten
			used = used || u
		}
		return out, used
	default:
		if encoded, err := json.Marshal(t); err == nil {
			if ref, ok := responseValues[strings.Trim(string(encoded), `"`)]; ok {
				return ref, true
			}
		}
		return t, false
	}
}

// collectResponseValues walks a JSON response body and records every leaf
// value's dotted path, so a LATER step's request can be checked against
// it. String leaves are recorded as-is; other JSON scalar leaves
// (numbers, bools) are recorded by their string form, since a request
// value ({{var}} interpolation always ends up as a string on the wire)
// needs to be compared against the same representation.
func collectResponseValues(id string, body []byte, out map[string]string) {
	if len(body) == 0 {
		return
	}
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return
	}
	walkJSONLeaves(parsed, "", func(path string, value any) {
		s, ok := scalarString(value)
		if !ok || s == "" {
			return
		}
		ref := fmt.Sprintf("{{responses.%s.body.%s}}", id, path)
		// First recorded step wins if the same literal value appears
		// under multiple paths/steps — an ambiguous match is still a
		// match, and picking the first-seen occurrence keeps recording
		// deterministic given the same input exchanges.
		if _, exists := out[s]; !exists {
			out[s] = ref
		}
	})
}

func scalarString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case float64, bool:
		encoded, err := json.Marshal(t)
		if err != nil {
			return "", false
		}
		return string(encoded), true
	default:
		return "", false
	}
}

// walkJSONLeaves visits every scalar leaf in a decoded JSON value,
// calling fn with its dotted path (matching the same dotted-path
// convention internal/assertions.lookupJSONPath and
// environment.lookupResponseJSONPath already use) and value.
func walkJSONLeaves(v any, path string, fn func(path string, value any)) {
	switch t := v.(type) {
	case map[string]any:
		for k, val := range t {
			childPath := k
			if path != "" {
				childPath = path + "." + k
			}
			walkJSONLeaves(val, childPath, fn)
		}
	case []any:
		for i, val := range t {
			childPath := fmt.Sprintf("%s.%d", path, i)
			if path == "" {
				childPath = fmt.Sprintf("%d", i)
			}
			walkJSONLeaves(val, childPath, fn)
		}
	case nil:
		// nothing to record
	default:
		fn(path, t)
	}
}
