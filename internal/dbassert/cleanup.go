// cleanup.go is the ONE place in this package that ever mutates a
// database. Every other file (mongo.go, dbassert.go) is read-only by
// design — dbassert.ValidateReadOnly exists specifically to reject a
// mutating SQL statement before it ever reaches a connection, and the
// MongoDB query methods (RowCountMongo, FirstValueMongo,
// ListCollectionNames) only ever call Find/CountDocuments/
// ListCollectionNames, never an insert/update/delete driver method.
//
// DeleteMongo below is the deliberate, narrow exception: it exists to
// support `cleanup:` blocks (domain.CleanupSpec) that delete a mutation
// test's own just-created record after the test finishes, so a test
// suite that exercises "create X" mutations does not leave permanent
// artifacts in a real database. It is reachable ONLY from
// testrunner.runOne's post-test cleanup step — never from anything
// compiled under `assert:`, which keeps the existing read-only guarantee
// for assertions fully intact.
package dbassert

import (
	"context"
	"fmt"
)

// DeleteMongo deletes every document in collection matching filter and
// returns how many were removed. Refuses to run against an empty/nil
// filter — an empty MongoDB filter matches (and would delete) every
// document in the collection, which is never what a teardown step
// intends. testdef.compileCleanup already rejects an empty filter at
// Compile time; this is a second, intentionally redundant guard so a
// caller that somehow bypasses compilation (e.g. a future direct
// dbassert consumer) cannot accidentally wipe a collection either.
func (r *Registry) DeleteMongo(ctx context.Context, connName, collection string, filter any) (int, error) {
	if isEmptyFilterValue(filter) {
		return 0, fmt.Errorf("cleanup.db.%s.%s: refusing to delete with an empty filter (this would delete every document in the collection)", connName, collection)
	}
	db, err := r.getMongo(connName)
	if err != nil {
		return 0, err
	}
	bsonFilter, err := toBSONFilter(filter)
	if err != nil {
		return 0, fmt.Errorf("cleanup.%s.filter: %w", connName, err)
	}
	res, err := db.Collection(collection).DeleteMany(ctx, bsonFilter)
	if err != nil {
		return 0, fmt.Errorf("cleaning up %q.%s: %w", connName, collection, err)
	}
	return int(res.DeletedCount), nil
}

// isEmptyFilterValue mirrors testdef's isEmptyFilter (duplicated rather
// than shared across the internal/testdef <-> internal/dbassert boundary,
// which has no existing dependency in either direction and shouldn't gain
// one for a three-line helper).
func isEmptyFilterValue(filter any) bool {
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
