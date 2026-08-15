package app

import (
	"path/filepath"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/sandeepv/apilens/internal/contract"
	"github.com/sandeepv/apilens/internal/openapigen"
)

// GenerateFromSpecOptions configures GenerateFromSpec (plan.md v9:
// "Automatic test generation from OpenAPI examples").
type GenerateFromSpecOptions struct {
	SpecPath string // required: path to a stored OpenAPI document
	Out      string // output directory; empty defaults to .apilens/tests/generated
	Force    bool
}

// GenerateFromSpecResult reports what GenerateFromSpec wrote.
type GenerateFromSpecResult struct {
	Files    []string
	Skipped  int // operations that had no usable example
	Compiled int // total operations declared in the spec
}

// GenerateFromSpec loads the OpenAPI document at opts.SpecPath (reusing
// internal/contract.Load — the exact same parse+validate path v7's
// contract testing already uses, so a spec good enough to contract-test
// against is good enough to generate from) and writes one YAML v1 test
// per operation that has a usable example (plan.md v9).
func (a *App) GenerateFromSpec(opts GenerateFromSpecOptions) (GenerateFromSpecResult, error) {
	doc, err := contract.Load(opts.SpecPath)
	if err != nil {
		return GenerateFromSpecResult{}, err
	}

	generated := openapigen.FromDoc(doc)
	totalOps := countOperations(doc)

	out := opts.Out
	if out == "" {
		out = filepath.Join(a.ProjectDir, ".apilens", "tests", "generated")
	}

	written, err := openapigen.WriteAll(generated, openapigen.WriteOptions{Dir: out, Force: opts.Force})
	if err != nil {
		return GenerateFromSpecResult{Files: written}, err
	}
	return GenerateFromSpecResult{
		Files:    written,
		Skipped:  totalOps - len(generated),
		Compiled: totalOps,
	}, nil
}

// countOperations counts every GET/POST/PUT/PATCH/DELETE operation in doc
// — used only to report how many were skipped for lacking an example,
// not for generation itself (openapigen.FromDoc does its own walk).
func countOperations(doc *openapi3.T) int {
	if doc == nil || doc.Paths == nil {
		return 0
	}
	count := 0
	for _, p := range doc.Paths.InMatchingOrder() {
		item := doc.Paths.Find(p)
		if item == nil {
			continue
		}
		for _, op := range []*openapi3.Operation{item.Get, item.Post, item.Put, item.Patch, item.Delete} {
			if op != nil {
				count++
			}
		}
	}
	return count
}
