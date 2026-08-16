// dbhints.go implements the __typename-based db.* auto-assertion feature
// discussed for `apilens generate`: a GraphQL response's "__typename"
// field names the GraphQL type of each returned object, and — for any
// object that also carries an "_id" — that pair is a candidate to verify
// against a real MongoDB collection ("Advance" -> try "advances").
//
// This is deliberately NOT "read the source code and figure out what a
// mutation touches" (docs/07-discovery.md's founding rule: discovery
// only ever reports what static analysis of source/schema can prove,
// never a guess). It only ever looks at the response body ApilLens
// already captured, and it never writes an assertion for a guessed
// collection name that isn't actually verified against
// dbassert.Registry.ListCollectionNames — an unverified guess is surfaced
// to the caller as a skipped hint instead of being silently written into
// the generated YAML (mirrors dbassert's own "we never invent" posture
// toward mutating SQL, applied here to "never invent a Mongo collection
// name that isn't in the database").
package generate

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

// dbRef is one candidate (GraphQL type, document id) pair pulled from a
// response body.
type dbRef struct {
	Typename string
	ID       string
}

// extractDBRefs walks a JSON response body and collects every unique
// (typename, id) pair found on an object that has both a string
// "__typename" and a string "_id" field. Map key order in encoding/json's
// decoded `any` tree is randomized by Go's map iteration — walking keys
// in sorted order keeps the returned slice's order deterministic across
// repeated runs on the same input (important so re-generating a test from
// the same capture produces byte-identical YAML, matching
// docs/07-discovery.md's "endpoints found are ordered and previously-seen
// results survive process restarts" discipline applied to this ordering
// question too).
func extractDBRefs(body []byte) []dbRef {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}
	var out []dbRef
	seen := map[string]bool{}
	walkForDBRefs(parsed, &out, seen)
	return out
}

func walkForDBRefs(node any, out *[]dbRef, seen map[string]bool) {
	switch v := node.(type) {
	case map[string]any:
		if typename, id := typenameAndID(v); typename != "" && id != "" {
			key := typename + "|" + id
			if !seen[key] {
				seen[key] = true
				*out = append(*out, dbRef{Typename: typename, ID: id})
			}
		}
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkForDBRefs(v[k], out, seen)
		}
	case []any:
		for _, item := range v {
			walkForDBRefs(item, out, seen)
		}
	}
}

// typenameAndID extracts "__typename" and "_id" as strings from a decoded
// JSON object, returning ("", "") if either is missing or not a string
// (e.g. a numeric _id — this feature only supports the string-hex-id
// convention every MongoDB-over-GraphQL response uses).
func typenameAndID(m map[string]any) (typename, id string) {
	t, _ := m["__typename"].(string)
	i, _ := m["_id"].(string)
	return t, i
}

// pluralizeTypename makes a best-effort English plural of a GraphQL type
// name, lowercased, as a guess at the Mongoose/MongoDB collection name
// backing it (e.g. "Advance" -> "advances", "Category" -> "categories").
// This is intentionally naive — real pluralization has plenty of
// irregular forms (Person -> People) this does not handle — which is
// exactly why the guess is always verified against
// dbassert.Registry.ListCollectionNames before being trusted (see
// enrichWithDBHints) rather than written into a generated test on faith.
func pluralizeTypename(typename string) string {
	lower := strings.ToLower(typename)
	switch {
	case strings.HasSuffix(lower, "y") && len(lower) > 1 && !isVowelByte(lower[len(lower)-2]):
		return lower[:len(lower)-1] + "ies"
	case strings.HasSuffix(lower, "s"), strings.HasSuffix(lower, "x"), strings.HasSuffix(lower, "z"),
		strings.HasSuffix(lower, "ch"), strings.HasSuffix(lower, "sh"):
		return lower + "es"
	default:
		return lower + "s"
	}
}

func isVowelByte(b byte) bool {
	switch b {
	case 'a', 'e', 'i', 'o', 'u':
		return true
	default:
		return false
	}
}

// CollectionLister is the minimal capability enrichWithDBHints needs —
// implemented by *dbassert.Registry (its ListCollectionNames method
// already has this exact signature) without internal/generate needing to
// import internal/dbassert at all, keeping the dependency direction the
// same "generate doesn't know how databases work" shape it already had.
type CollectionLister interface {
	ListCollectionNames(connName string) ([]string, error)
}

// DBHintOptions enables __typename-based db.* auto-assertion generation.
// Connection must name a connection already configured in config.yaml
// (db.connections.<Connection>) — enrichWithDBHints never invents one.
type DBHintOptions struct {
	Connection string
	Lister     CollectionLister
}

// enrichWithDBHints derives assert.db checks from ex's response body and
// appends them to tc.Assert.DB[hints.Connection]. Every derived check's
// collection name has been confirmed to actually exist in the connected
// database via hints.Lister — a (typename, id) pair whose guessed
// collection name doesn't match anything real is reported back as a
// skipped-hint message instead of being written into the test, so the
// generated YAML never asserts against a collection that was only ever a
// guess. Returns the (possibly unchanged) TestCase and the list of
// skipped-hint messages (nil if there were none or nothing to enrich).
func enrichWithDBHints(tc domain.TestCase, ex domain.Exchange, hints DBHintOptions) (domain.TestCase, []string) {
	refs := extractDBRefs(ex.Response.Body)
	if len(refs) == 0 {
		return tc, nil
	}

	names, err := hints.Lister.ListCollectionNames(hints.Connection)
	if err != nil {
		return tc, []string{fmt.Sprintf("db hints: could not list collections on connection %q: %v", hints.Connection, err)}
	}
	realByLower := make(map[string]string, len(names))
	for _, n := range names {
		realByLower[strings.ToLower(n)] = n
	}

	var specs []domain.DBSpec
	var skipped []string
	for _, ref := range refs {
		candidate := pluralizeTypename(ref.Typename)
		real, ok := realByLower[candidate]
		if !ok {
			// Fall back to the unpluralized lowercase type name, in case
			// this schema names its collections after the singular type
			// (e.g. type "User" -> collection "user").
			real, ok = realByLower[strings.ToLower(ref.Typename)]
		}
		if !ok {
			skipped = append(skipped, fmt.Sprintf(
				"%s (_id=%s): no collection named %q (or %q) exists on connection %q — add a db.%s block by hand if this should be checked",
				ref.Typename, ref.ID, candidate, strings.ToLower(ref.Typename), hints.Connection, hints.Connection))
			continue
		}
		exists := true
		specs = append(specs, domain.DBSpec{
			Collection: real,
			Filter:     map[string]any{"_id": ref.ID},
			Exists:     &exists,
		})
	}

	if len(specs) > 0 {
		if tc.Assert.DB == nil {
			tc.Assert.DB = map[string][]domain.DBSpec{}
		}
		tc.Assert.DB[hints.Connection] = append(tc.Assert.DB[hints.Connection], specs...)
	}
	return tc, skipped
}
