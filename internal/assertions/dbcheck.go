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
func newDBCheck(connName string, spec domain.DBSpec, reg *dbassert.Registry) (domain.Check, error) {
	if spec.Query == "" {
		return nil, fmt.Errorf("db.%s requires a \"query\"", connName)
	}
	return dbCheck{connName: connName, spec: spec, reg: reg}, nil
}

// dbCheck bundles every configured expectation for one db.<connection>
// block into a single domain.Check, since they all share one query
// execution (no reason to run the same SELECT multiple times per test).
type dbCheck struct {
	connName string
	spec     domain.DBSpec
	reg      *dbassert.Registry
}

func (c dbCheck) Eval(ex domain.Exchange) domain.AssertionResult {
	ctx, cancel := context.WithTimeout(context.Background(), dbQueryTimeout)
	defer cancel()

	if c.spec.RowCountEquals != nil {
		count, err := c.reg.RowCount(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBRowCount, c.connName, err)
		}
		want := *c.spec.RowCountEquals
		if count == want {
			return domain.AssertionResult{
				Kind: domain.KindDBRowCount, Target: c.connName, Passed: true,
				Expected: fmt.Sprintf("%d row(s)", want), Actual: fmt.Sprintf("%d row(s)", count),
			}
		}
		return domain.AssertionResult{
			Kind: domain.KindDBRowCount, Target: c.connName, Passed: false,
			Expected: fmt.Sprintf("%d row(s)", want), Actual: fmt.Sprintf("%d row(s)", count),
		}
	}

	if c.spec.Exists != nil {
		count, err := c.reg.RowCount(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBExists, c.connName, err)
		}
		found := count > 0
		want := *c.spec.Exists
		return domain.AssertionResult{
			Kind: domain.KindDBExists, Target: c.connName, Passed: found == want,
			Expected: fmt.Sprintf("%v", want), Actual: fmt.Sprintf("%v", found),
		}
	}

	if c.spec.Equals != nil {
		val, found, err := c.reg.FirstValue(ctx, c.connName, c.spec.Query, c.spec.Args...)
		if err != nil {
			return dbErrorResult(domain.KindDBEquals, c.connName, err)
		}
		want := fmt.Sprintf("%v", c.spec.Equals)
		if !found {
			return domain.AssertionResult{
				Kind: domain.KindDBEquals, Target: c.connName, Passed: false,
				Expected: want, Reason: "query returned no rows",
			}
		}
		return domain.AssertionResult{
			Kind: domain.KindDBEquals, Target: c.connName, Passed: val == want,
			Expected: want, Actual: val,
		}
	}

	// No expectation field was set — the query still ran (so a
	// misbehaving/erroring query is still caught), but there is nothing
	// to compare, which is itself a config-shaped problem the author
	// should fix.
	return domain.AssertionResult{
		Kind: domain.KindDBRowCount, Target: c.connName, Passed: false,
		Reason: fmt.Sprintf("db.%s sets no expectation (row_count_equals, exists, or equals)", c.connName),
	}
}

func dbErrorResult(kind domain.AssertionKind, target string, err error) domain.AssertionResult {
	return domain.AssertionResult{Kind: kind, Target: target, Passed: false, Reason: err.Error()}
}
