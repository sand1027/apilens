package assertions

import (
	"context"
	"fmt"
	"time"

	"github.com/sandeepv/apilens/internal/dbassert"
	"github.com/sandeepv/apilens/internal/domain"
)

// dbQueryTimeout bounds a single db.* query so a hung database can't
// stall a test suite indefinitely — the same "no unbounded external
// call" discipline internal/runner already applies to HTTP requests
// (which have their own per-request timeout), extended to SQL.
const dbQueryTimeout = 5 * time.Second

// newDBCheck picks which db.* check to build based on which field of
// DBSpec is set. Exactly one of RowCountEquals/Exists/Equals is expected
// to be meaningfully checkable per compiled check; if more than one is
// set, all are evaluated and ALL must pass (same "every set field is
// independently checked" convention json.* assertions already use).
// testdef.compileAssert already guarantees exactly one of Query/Collection
// is set before this is ever called.
func newDBCheck(connName string, spec domain.DBSpec, reg *dbassert.Registry) (domain.Check, error) {
	if spec.Query == "" && spec.Collection == "" {
		return nil, fmt.Errorf("db.%s requires either \"query\" or \"collection\"", connName)
	}
	return dbCheck{connName: connName, spec: spec, reg: reg}, nil
}

// dbCheck bundles every configured expectation for one db.<connection>
// entry into a single domain.Check, since they all share one query
// execution (no reason to run the same SELECT multiple times per test).
// A test's db.<connection> may hold several dbCheck values now (one per
// list entry) — target() gives each a distinguishing report label so
// e.g. two checks against the same "main" connection but different
// collections don't look identical in output.
type dbCheck struct {
	connName string
	spec     domain.DBSpec
	reg      *dbassert.Registry
}

// target labels this check's AssertionResult.Target as "<connection>" for
// the SQL shape (a query has no single natural short name) or
// "<connection>.<collection>" for the MongoDB shape, so a report with
// multiple db checks against the same connection (e.g. one per collection
// a mutation wrote to) shows which one is which instead of N identical
// "db.exists: main" lines.
func (c dbCheck) target() string {
	if c.spec.Collection != "" {
		return c.connName + "." + c.spec.Collection
	}
	return c.connName
}

func (c dbCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
	defer cancel()

	if c.spec.Collection != "" {
		return c.evalMongo(ctx)
	}
	return c.evalSQL(ctx)
}

// evalSQL handles the sqlite/postgres shape (Query/Args).
func (c dbCheck) evalSQL(ctx context.Context) domain.AssertionResult {
	target := c.target()
	if c.spec.RowCountEquals != nil {
		count, err := c.reg.RowCount(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBRowCount, target, err)
		}
		want := *c.spec.RowCountEquals
		if count == want {
			return domain.AssertionResult{
				Kind: domain.KindDBRowCount, Target: target, Passed: true,
				Expected: fmt.Sprintf("%d row(s)", want), Actual: fmt.Sprintf("%d row(s)", count),
			}
		}
		return domain.AssertionResult{
			Kind: domain.KindDBRowCount, Target: target, Passed: false,
			Expected: fmt.Sprintf("%d row(s)", want), Actual: fmt.Sprintf("%d row(s)", count),
		}
	}

	if c.spec.Exists != nil {
		count, err := c.reg.RowCount(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBExists, target, err)
		}
		found := count > 0
		want := *c.spec.Exists
		return domain.AssertionResult{
			Kind: domain.KindDBExists, Target: target, Passed: found == want,
			Expected: fmt.Sprintf("%v", want), Actual: fmt.Sprintf("%v", found),
		}
	}

	if c.spec.Equals != nil {
		val, found, err := c.reg.FirstValue(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBEquals, target, err)
		}
		want := fmt.Sprintf("%v", c.spec.Equals)
		if !found {
			return domain.AssertionResult{
				Kind: domain.KindDBEquals, Target: target, Passed: false,
				Expected: want, Reason: "query returned no rows",
			}
		}
		return domain.AssertionResult{
			Kind: domain.KindDBEquals, Target: target, Passed: val == want,
			Expected: want, Actual: val,
		}
	}

	// No expectation field was set — the query still ran (so a
	// misbehaving/erroring query is still caught), but there is nothing
	// to compare, which is itself a config-shaped problem the author
	// should fix.
	return domain.AssertionResult{
		Kind: domain.KindDBRowCount, Target: target, Passed: false,
		Reason: fmt.Sprintf("db.%s sets no expectation (row_count_equals, exists, or equals)", target),
	}
}

// evalMongo handles the MongoDB shape (Collection/Filter/Field). Mirrors
// evalSQL's structure exactly, just calling the *Mongo registry methods
// instead — every AssertionKind/reporting shape stays identical between
// the two so db.* results look the same in reports regardless of which
// database engine is behind a connection.
func (c dbCheck) evalMongo(ctx context.Context) domain.AssertionResult {
	target := c.target()
	if c.spec.RowCountEquals != nil {
		count, err := c.reg.RowCountMongo(ctx, c.connName, c.spec.Collection, c.spec.Filter)
		if err != nil {
			return dbErrorResult(domain.KindDBRowCount, target, err)
		}
		want := *c.spec.RowCountEquals
		return domain.AssertionResult{
			Kind: domain.KindDBRowCount, Target: target, Passed: count == want,
			Expected: fmt.Sprintf("%d document(s)", want), Actual: fmt.Sprintf("%d document(s)", count),
		}
	}

	if c.spec.Exists != nil {
		count, err := c.reg.RowCountMongo(ctx, c.connName, c.spec.Collection, c.spec.Filter)
		if err != nil {
			return dbErrorResult(domain.KindDBExists, target, err)
		}
		found := count > 0
		want := *c.spec.Exists
		return domain.AssertionResult{
			Kind: domain.KindDBExists, Target: target, Passed: found == want,
			Expected: fmt.Sprintf("%v", want), Actual: fmt.Sprintf("%v", found),
		}
	}

	if c.spec.Equals != nil {
		val, found, err := c.reg.FirstValueMongo(ctx, c.connName, c.spec.Collection, c.spec.Filter, c.spec.Field)
		if err != nil {
			return dbErrorResult(domain.KindDBEquals, target, err)
		}
		want := fmt.Sprintf("%v", c.spec.Equals)
		if !found {
			return domain.AssertionResult{
				Kind: domain.KindDBEquals, Target: target, Passed: false,
				Expected: want, Reason: "filter matched no documents",
			}
		}
		return domain.AssertionResult{
			Kind: domain.KindDBEquals, Target: target, Passed: val == want,
			Expected: want, Actual: val,
		}
	}

	return domain.AssertionResult{
		Kind: domain.KindDBRowCount, Target: target, Passed: false,
		Reason: fmt.Sprintf("db.%s sets no expectation (row_count_equals, exists, or equals)", target),
	}
}

func dbErrorResult(kind domain.AssertionKind, target string, err error) domain.AssertionResult {
	return domain.AssertionResult{Kind: kind, Target: target, Passed: false, Reason: err.Error()}
}
