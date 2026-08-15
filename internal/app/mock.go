package app

import (
	"context"
	"strconv"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/mock"
	"github.com/sandeepv/apilens/internal/pagemap"
	"github.com/sandeepv/apilens/internal/registry"
)

// MockOptions configures Mock (docs/09-security.md section 5's loopback
// bind policy, same shape as WatchOptions).
type MockOptions struct {
	Bind        string
	Port        int
	AllowRemote bool
}

// MockHandle is returned by Mock so the CLI can print the bound address.
type MockHandle struct {
	Addr string
}

// PageMap implements plan.md v8's "Page-to-API mapping": groups history
// by the Referer header captured on each exchange (see internal/pagemap's
// package doc for the honesty note this carries forward from
// docs/11-risks-and-gaps.md G19 — the mapping is inferred, not ground
// truth).
func (a *App) PageMap() []pagemap.PageMapping {
	return pagemap.Build(a.HistoryList(0))
}

// Mock starts a local mock server (plan.md v8: "API mocking (apilens
// mock) using registry + captured examples") over the current registry,
// using the most recent captured exchange per endpoint (same
// latestExchangePerEndpoint helper internal/app/contract.go's SpecExport
// and ContractTest use, so all three v7/v8 features share one notion of
// "the best known example for this endpoint"). Blocks internally until
// ctx is canceled, same lifecycle as Watch.
func (a *App) Mock(ctx context.Context, opts MockOptions) (*MockHandle, error) {
	endpoints := a.Registry.List(registry.Filter{})
	if len(endpoints) == 0 {
		return nil, domain.NewNotFoundError(
			"registry is empty — run apilens discover (or apilens watch) first")
	}
	examples := a.latestExchangePerEndpoint(endpoints)

	srv := mock.New(endpoints, examples)
	bind := resolveMockBind(opts)
	if err := srv.Bind(mock.Options{Bind: bind, AllowRemote: opts.AllowRemote}); err != nil {
		return nil, err
	}

	go func() { _ = srv.Serve(ctx) }()

	return &MockHandle{Addr: srv.Addr()}, nil
}

func resolveMockBind(opts MockOptions) string {
	bind := opts.Bind
	if bind == "" {
		bind = "127.0.0.1"
	}
	port := opts.Port
	if port == 0 {
		port = 4489
	}
	return bind + ":" + strconv.Itoa(port)
}
