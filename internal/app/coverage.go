package app

import (
	"net/url"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/graphqlop"
	"github.com/sandeepv/apilens/internal/registry"
	"github.com/sandeepv/apilens/internal/testdef"
)

// CoverageResult is what `apilens coverage` reports: every discovered
// endpoint (from the last `apilens discover`), matched against which
// compiled test files exercise it. Not itself a Report/TestResult — this
// never runs a request, it only cross-references two things already on
// disk (the registry cache and .apilens/tests/*.yaml), which is why it
// can run in a fraction of a second even against hundreds of discovered
// operations.
type CoverageResult struct {
	Total   int
	Covered []CoverageEndpoint
	Missing []CoverageEndpoint
}

// CoverageEndpoint is one discovered endpoint plus the test file(s) (if
// any) that exercise it.
type CoverageEndpoint struct {
	Method string
	Path   string
	Tests  []string // test Names, empty if uncovered
}

// Percent is Covered/Total*100, 0 when there are no discovered endpoints
// at all (matches domain.Counts.SuccessPercent's zero-denominator
// convention elsewhere in this codebase).
func (c CoverageResult) Percent() float64 {
	if c.Total == 0 {
		return 0
	}
	return float64(len(c.Covered)) / float64(c.Total) * 100
}

// Coverage cross-references the endpoint registry (populated by the last
// `apilens discover`) against every compiled test under .apilens/tests,
// reporting which discovered operations have at least one test and which
// have none. Endpoints are matched by the SAME domain.EndpointID identity
// discover/list/inspect already use — normalize(method)+normalize(path)
// — so a test matches an endpoint if and only if `apilens test <ref>`
// would also have resolved to it, keeping this command's notion of
// "covered" consistent with the rest of the CLI rather than inventing a
// second identity scheme.
func (a *App) Coverage() (CoverageResult, error) {
	endpoints := a.Registry.List(registry.Filter{})

	testsDir := filepath.Join(a.ProjectDir, ".apilens", "tests")
	loader := testdef.NewLoader()
	tests, err := loader.LoadAll(testsDir)
	if err != nil {
		return CoverageResult{}, err
	}

	// GraphQL operations match by exact EndpointID (their registry path
	// already IS the concrete field name, e.g. /graphql/mutation/
	// createAdvance — there is no ":param" segment to reconcile). REST
	// endpoints need param-aware matching instead: a registry path like
	// /api/users/{id} must match a test hitting the CONCRETE
	// /api/users/1, and NormalizePath alone can't tell "1" apart from a
	// literal path segment — only pathMatchesPattern (segment-by-segment,
	// mirroring internal/mock's own route matching) can.
	graphqlTestsByID := make(map[domain.EndpointID][]string)
	var restTests []restCoverageCandidate
	for _, tc := range tests {
		if tc.Request.GraphQL != nil {
			id, ok := graphqlEndpointID(tc)
			if !ok {
				continue
			}
			graphqlTestsByID[id] = append(graphqlTestsByID[id], tc.Name)
			continue
		}
		cand, ok := restCoverageCandidateFor(tc)
		if !ok {
			continue
		}
		restTests = append(restTests, cand)
	}

	result := CoverageResult{Total: len(endpoints)}
	for _, ep := range endpoints {
		var names []string
		if isGraphQLPath(ep.Path) {
			names = graphqlTestsByID[domain.NewEndpointID(ep.Method, ep.Path)]
		} else {
			names = restTestNamesCovering(ep, restTests)
		}
		sort.Strings(names)
		ce := CoverageEndpoint{Method: string(ep.Method), Path: ep.Path, Tests: names}
		if len(names) > 0 {
			result.Covered = append(result.Covered, ce)
		} else {
			result.Missing = append(result.Missing, ce)
		}
	}
	sort.Slice(result.Covered, func(i, j int) bool { return coverageLess(result.Covered[i], result.Covered[j]) })
	sort.Slice(result.Missing, func(i, j int) bool { return coverageLess(result.Missing[i], result.Missing[j]) })
	return result, nil
}

func coverageLess(a, b CoverageEndpoint) bool {
	if a.Path != b.Path {
		return a.Path < b.Path
	}
	return a.Method < b.Method
}

func isGraphQLPath(path string) bool {
	return strings.HasPrefix(path, "/graphql/")
}

// graphqlEndpointID computes the same domain.EndpointID a discovered
// GraphQL registry entry would have, from a compiled test's own request
// template — WITHOUT sending it (Coverage never makes a network call).
// Returns ok=false when the query doesn't parse (e.g. it's actually a
// chained/templated string not valid on its own) — such a test simply
// doesn't count toward coverage rather than erroring the whole command.
func graphqlEndpointID(tc domain.TestCase) (domain.EndpointID, bool) {
	query := tc.Request.GraphQL.Query
	if strings.TrimSpace(query) == "" {
		return "", false
	}
	op, err := graphqlop.ParseQuery(query)
	if err != nil {
		return "", false
	}
	field := op.PrimaryField()
	path := graphqlop.RegistryPath(op.Type, field)
	method := graphqlop.DisplayMethod(op.Type)
	return domain.NewEndpointID(method, path), true
}

// restCoverageCandidate is a REST test reduced to just what coverage
// matching needs: its method and path segments (query string and host
// already stripped).
type restCoverageCandidate struct {
	method   domain.Method
	segments []string
	testName string
}

// restCoverageCandidateFor resolves a REST test's URL template to
// (method, path segments), ignoring any query string/host and NOT
// expanding "{{base_url}}" (coverage matches by path only, same as the
// registry's own identity, which never encodes a host) — only
// "{{base_url}}" is stripped as a known prefix; any OTHER "{{var}}" left
// in the path (e.g. a chained "{{responses...}}" id, or a legitimate
// runtime path param the author templated) makes this test unresolvable
// STATICALLY, so it is excluded from coverage rather than matched against
// a path that still contains a literal "{{...}}" placeholder.
func restCoverageCandidateFor(tc domain.TestCase) (restCoverageCandidate, bool) {
	raw := strings.TrimSpace(tc.Request.URL)
	if raw == "" {
		return restCoverageCandidate{}, false
	}
	raw = strings.TrimPrefix(raw, "{{base_url}}")
	if strings.Contains(raw, "{{") {
		return restCoverageCandidate{}, false
	}
	u, err := url.Parse(raw)
	if err != nil {
		return restCoverageCandidate{}, false
	}
	path := u.Path
	if path == "" {
		path = "/"
	}
	method := tc.Request.Method
	if method == "" {
		method = "GET"
	}
	return restCoverageCandidate{
		method:   domain.NormalizeMethod(string(method)),
		segments: splitPathSegments(path),
		testName: tc.Name,
	}, true
}

// restTestNamesCovering returns every test name whose (method, concrete
// path) matches ep's (method, possibly-parameterized) pattern.
func restTestNamesCovering(ep domain.Endpoint, candidates []restCoverageCandidate) []string {
	patternSegs := splitPathSegments(ep.Path)
	var names []string
	for _, c := range candidates {
		if c.method != ep.Method {
			continue
		}
		if pathSegmentsMatch(patternSegs, c.segments) {
			names = append(names, c.testName)
		}
	}
	return names
}

// pathSegmentsMatch mirrors internal/mock.scoreMatch's route-matching
// rule (duplicated rather than imported — internal/app already sits
// above internal/mock in the dependency graph per docs/02-packages.md,
// and pulling a request-serving package's route matcher into a read-only
// reporting command is the wrong direction; this is 10 lines, not worth
// a shared package for): equal length, and every segment either matches
// literally or the PATTERN segment is a param placeholder ("{name}" or
// ":name" — discovery emits :id-style via NormalizePath, but a raw
// OpenAPI/registry path can still carry the original {param} spelling, so
// both forms are treated as a wildcard here).
func pathSegmentsMatch(pattern, candidate []string) bool {
	if len(pattern) != len(candidate) {
		return false
	}
	for i := range pattern {
		if pattern[i] == candidate[i] {
			continue
		}
		if isPathParamSegment(pattern[i]) {
			continue
		}
		return false
	}
	return true
}

func isPathParamSegment(seg string) bool {
	if strings.HasPrefix(seg, ":") {
		return true
	}
	return strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
}

func splitPathSegments(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return []string{}
	}
	return strings.Split(p, "/")
}
