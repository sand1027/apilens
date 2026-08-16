package generate

import (
	"fmt"
	"sort"
	"testing"

	"github.com/sandeepv/apilens/internal/domain"
)

func TestPluralizeTypename(t *testing.T) {
	cases := map[string]string{
		"Advance":  "advances",
		"Receipt":  "receipts",
		"Payment":  "payments",
		"Category": "categories",
		"Address":  "addresses", // ends in "s" -> +es
		"Box":      "boxes",     // ends in "x" -> +es
		"Match":    "matches",   // ends in "ch" -> +es
		"Wish":     "wishes",    // ends in "sh" -> +es
		"User":     "users",
		"Buzz":     "buzzes", // ends in "z" -> +es
	}
	for in, want := range cases {
		if got := pluralizeTypename(in); got != want {
			t.Errorf("pluralizeTypename(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestExtractDBRefs_FindsTopLevelObject(t *testing.T) {
	body := []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4","total":20000}}}`)
	refs := extractDBRefs(body)
	if len(refs) != 1 {
		t.Fatalf("expected 1 ref, got %d: %+v", len(refs), refs)
	}
	if refs[0].Typename != "Advance" || refs[0].ID != "6a81dcfa6f6ebb3f76b802d4" {
		t.Errorf("ref = %+v", refs[0])
	}
}

func TestExtractDBRefs_FindsNestedObjects(t *testing.T) {
	// Mirrors the real createAdvance response captured this session:
	// Advance -> receipt (Receipt) -> payment (Payment). Payment here
	// has no _id in the captured shape, so only Advance and Receipt
	// should be extracted.
	body := []byte(`{
		"data": {
			"createAdvance": {
				"__typename": "Advance",
				"_id": "6a81dcfa6f6ebb3f76b802d4",
				"receipt": {
					"__typename": "Receipt",
					"_id": "6a81dcfa6f6ebb3f76b802d9",
					"payment": {
						"__typename": "Payment",
						"mode": "CARD"
					}
				}
			}
		}
	}`)
	refs := extractDBRefs(body)
	byTypename := map[string]string{}
	for _, r := range refs {
		byTypename[r.Typename] = r.ID
	}
	if byTypename["Advance"] != "6a81dcfa6f6ebb3f76b802d4" {
		t.Errorf("Advance ref missing or wrong: %+v", refs)
	}
	if byTypename["Receipt"] != "6a81dcfa6f6ebb3f76b802d9" {
		t.Errorf("Receipt ref missing or wrong: %+v", refs)
	}
	if _, ok := byTypename["Payment"]; ok {
		t.Errorf("Payment has no _id in this response and must not be extracted, got %+v", refs)
	}
}

func TestExtractDBRefs_FindsObjectsInsideArrays(t *testing.T) {
	body := []byte(`{"data":{"users":[
		{"__typename":"User","_id":"aaaaaaaaaaaaaaaaaaaaaaaa"},
		{"__typename":"User","_id":"bbbbbbbbbbbbbbbbbbbbbbbb"}
	]}}`)
	refs := extractDBRefs(body)
	if len(refs) != 2 {
		t.Fatalf("expected 2 refs, got %d: %+v", len(refs), refs)
	}
}

func TestExtractDBRefs_DeduplicatesRepeatedTypenameID(t *testing.T) {
	body := []byte(`{"data":{"a":{"__typename":"User","_id":"aaaaaaaaaaaaaaaaaaaaaaaa"},"b":{"__typename":"User","_id":"aaaaaaaaaaaaaaaaaaaaaaaa"}}}`)
	refs := extractDBRefs(body)
	if len(refs) != 1 {
		t.Fatalf("expected exactly 1 deduplicated ref, got %d: %+v", len(refs), refs)
	}
}

func TestExtractDBRefs_IgnoresObjectsMissingTypenameOrID(t *testing.T) {
	body := []byte(`{"data":{
		"onlyTypename": {"__typename": "Advance"},
		"onlyID": {"_id": "aaaaaaaaaaaaaaaaaaaaaaaa"},
		"neither": {"total": 100}
	}}`)
	refs := extractDBRefs(body)
	if len(refs) != 0 {
		t.Errorf("expected 0 refs when neither field pair is complete, got %+v", refs)
	}
}

func TestExtractDBRefs_NonJSONBodyReturnsNilNotPanic(t *testing.T) {
	refs := extractDBRefs([]byte("not json at all"))
	if refs != nil {
		t.Errorf("expected nil for non-JSON body, got %+v", refs)
	}
}

func TestExtractDBRefs_EmptyBodyReturnsNil(t *testing.T) {
	refs := extractDBRefs(nil)
	if refs != nil {
		t.Errorf("expected nil for empty body, got %+v", refs)
	}
}

func TestExtractDBRefs_DeterministicOrderAcrossRuns(t *testing.T) {
	body := []byte(`{"data":{"z":{"__typename":"Zebra","_id":"aaaaaaaaaaaaaaaaaaaaaaaa"},"a":{"__typename":"Apple","_id":"bbbbbbbbbbbbbbbbbbbbbbbb"}}}`)
	first := extractDBRefs(body)
	for i := 0; i < 5; i++ {
		again := extractDBRefs(body)
		if fmt.Sprint(again) != fmt.Sprint(first) {
			t.Fatalf("extractDBRefs order was not deterministic across repeated calls:\nfirst: %+v\nagain: %+v", first, again)
		}
	}
}

// fakeCollectionLister implements CollectionLister with a fixed,
// in-memory set of collection names, so enrichWithDBHints can be tested
// without a real MongoDB connection.
type fakeCollectionLister struct {
	names []string
	err   error
}

func (f fakeCollectionLister) ListCollectionNames(connName string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.names, nil
}

func TestEnrichWithDBHints_AddsVerifiedCollectionCheck(t *testing.T) {
	tc := domain.TestCase{Name: "Create advance"}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`),
	}}

	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{names: []string{"advances", "receipts", "users"}},
	})

	if len(skipped) != 0 {
		t.Errorf("expected no skipped hints, got %v", skipped)
	}
	specs, ok := got.Assert.DB["main"]
	if !ok || len(specs) != 1 {
		t.Fatalf("expected exactly 1 db check under connection main, got %+v", got.Assert.DB)
	}
	if specs[0].Collection != "advances" {
		t.Errorf("Collection = %q, want advances", specs[0].Collection)
	}
	filter, ok := specs[0].Filter.(map[string]any)
	if !ok || filter["_id"] != "6a81dcfa6f6ebb3f76b802d4" {
		t.Errorf("Filter = %#v", specs[0].Filter)
	}
	if specs[0].Exists == nil || !*specs[0].Exists {
		t.Errorf("Exists = %v, want true", specs[0].Exists)
	}
}

func TestEnrichWithDBHints_SkipsUnverifiedGuessWithoutWritingIt(t *testing.T) {
	tc := domain.TestCase{Name: "Create something"}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{"data":{"createThing":{"__typename":"ExoticWidget","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`),
	}}

	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		// Neither "exoticwidgets" nor "exoticwidget" exists.
		Lister: fakeCollectionLister{names: []string{"advances", "receipts"}},
	})

	if len(skipped) != 1 {
		t.Fatalf("expected exactly 1 skipped hint, got %v", skipped)
	}
	if len(got.Assert.DB) != 0 {
		t.Errorf("expected no db checks written for an unverified guess, got %+v", got.Assert.DB)
	}
}

func TestEnrichWithDBHints_FallsBackToSingularCollectionName(t *testing.T) {
	tc := domain.TestCase{Name: "Create session"}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{"data":{"createSession":{"__typename":"Session","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`),
	}}

	// Collection is named "session" (singular), not the pluralized
	// "sessions" -- must still be found via the singular fallback.
	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{names: []string{"session"}},
	})

	if len(skipped) != 0 {
		t.Errorf("expected no skipped hints, got %v", skipped)
	}
	specs := got.Assert.DB["main"]
	if len(specs) != 1 || specs[0].Collection != "session" {
		t.Errorf("expected a check against the singular collection name, got %+v", specs)
	}
}

func TestEnrichWithDBHints_NoRefsInResponseIsNoOp(t *testing.T) {
	tc := domain.TestCase{Name: "Ping"}
	ex := domain.Exchange{Response: domain.HTTPResponse{Body: []byte(`{"data":{"ping":{"message":"pong"}}}`)}}

	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{names: []string{"advances"}},
	})

	if len(skipped) != 0 {
		t.Errorf("expected no skipped hints when there is nothing to check, got %v", skipped)
	}
	if len(got.Assert.DB) != 0 {
		t.Errorf("expected no db checks added, got %+v", got.Assert.DB)
	}
}

func TestEnrichWithDBHints_ListerErrorSurfacesAsSkippedHintNotPanic(t *testing.T) {
	tc := domain.TestCase{Name: "Create advance"}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`),
	}}

	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{err: fmt.Errorf("connection refused")},
	})

	if len(skipped) != 1 {
		t.Fatalf("expected exactly 1 skipped-hint message describing the lister error, got %v", skipped)
	}
	if len(got.Assert.DB) != 0 {
		t.Errorf("expected no db checks when the lister itself failed, got %+v", got.Assert.DB)
	}
}

func TestEnrichWithDBHints_MultipleRefsProduceMultipleChecksInStableOrder(t *testing.T) {
	tc := domain.TestCase{Name: "Create advance"}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{
			"data": {
				"createAdvance": {
					"__typename": "Advance",
					"_id": "6a81dcfa6f6ebb3f76b802d4",
					"receipt": {"__typename": "Receipt", "_id": "6a81dcfa6f6ebb3f76b802d9"}
				}
			}
		}`),
	}}

	got, skipped := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{names: []string{"advances", "receipts"}},
	})

	if len(skipped) != 0 {
		t.Errorf("expected no skipped hints, got %v", skipped)
	}
	specs := got.Assert.DB["main"]
	if len(specs) != 2 {
		t.Fatalf("expected 2 db checks, got %+v", specs)
	}
	collections := []string{specs[0].Collection, specs[1].Collection}
	sort.Strings(collections)
	if collections[0] != "advances" || collections[1] != "receipts" {
		t.Errorf("collections = %v, want [advances receipts]", collections)
	}
}

func TestEnrichWithDBHints_PreservesExistingDBChecksRatherThanOverwriting(t *testing.T) {
	handWritten := true
	tc := domain.TestCase{
		Name: "Create advance",
		Assert: domain.AssertionSpec{
			DB: map[string][]domain.DBSpec{
				"main": {{Collection: "ledgers", Filter: map[string]any{"source": "x"}, Exists: &handWritten}},
			},
		},
	}
	ex := domain.Exchange{Response: domain.HTTPResponse{
		Body: []byte(`{"data":{"createAdvance":{"__typename":"Advance","_id":"6a81dcfa6f6ebb3f76b802d4"}}}`),
	}}

	got, _ := enrichWithDBHints(tc, ex, DBHintOptions{
		Connection: "main",
		Lister:     fakeCollectionLister{names: []string{"advances"}},
	})

	specs := got.Assert.DB["main"]
	if len(specs) != 2 {
		t.Fatalf("expected the hand-written check plus the new derived one (2 total), got %+v", specs)
	}
	foundLedgers, foundAdvances := false, false
	for _, s := range specs {
		if s.Collection == "ledgers" {
			foundLedgers = true
		}
		if s.Collection == "advances" {
			foundAdvances = true
		}
	}
	if !foundLedgers || !foundAdvances {
		t.Errorf("expected both the pre-existing ledgers check and the new advances check, got %+v", specs)
	}
}
