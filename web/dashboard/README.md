# ApiLens dashboard (v6)

`apilens ui` serves a single-page Next.js app, statically exported and
embedded into the `apilens` binary via `go:embed` (see `embed.go`). There
is no separate Node process at runtime — the dashboard is entirely
self-contained in the compiled Go binary.

## Layout

```text
web/dashboard/
├── embed.go              # go:embed of frontend/out into the apilens binary
└── frontend/              # Next.js app (source of truth)
    ├── app/                # single route: app/page.tsx, tabs are client state
    ├── components/         # Explorer, RequestBuilder, History, Monitor, TestRunner, EnvironmentSwitcher, HitsHUD
    ├── lib/api.ts           # typed fetch wrapper around internal/webapi's REST+SSE API
    ├── next.config.ts       # output: "export" — static HTML/JS/CSS only
    └── out/                 # build output — committed, see below
```

## Why `out/` is committed

`out/` is normally build output and would be gitignored. It is
**intentionally checked in** here because `web/dashboard/embed.go` embeds
it directly into the Go binary. Committing it means `go build
./cmd/apilens` works on any machine with just a Go toolchain — no Node/npm
required to build ApiLens itself.

**If you change anything under `app/` or `components/`, you must
regenerate `out/` and commit it:**

```bash
cd web/dashboard/frontend
npm install   # first time only
npm run build
git add out/
```

## Local frontend development

Run the Next.js dev server against a separately-running `apilens ui`
backend:

```bash
# terminal 1
apilens ui --port 4488

# terminal 2
cd web/dashboard/frontend
NEXT_PUBLIC_API_BASE=http://127.0.0.1:4488 npm run dev
```

`NEXT_PUBLIC_API_BASE` is only needed because the dev server and the Go
backend run on different ports; in the embedded production build they are
same-origin and this is unset.

## Architecture notes

- **Single page, client-side tabs.** All five dashboard views (Explorer,
  Request Builder, History, Runtime Monitor, Tests) are one Next.js route
  with tab state in React, not separate pages. This sidesteps a known
  Next.js 16 static-export multi-route RSC path issue and keeps the
  embedded output trivial to serve.
- **No business logic here.** Every component calls `lib/api.ts`, which
  calls `internal/webapi`'s REST/SSE endpoints, which call
  `pkg/apilens.Engine` — the same Engine the CLI uses. If a dashboard
  action doesn't map to an existing Engine method, it doesn't belong here
  (plan.md v6: "Every UI action is an Engine method that the CLI can
  already perform").
- **`go:embed all:frontend/out`**, not `go:embed frontend/out` — the
  `all:` prefix is required because plain `go:embed` silently drops any
  path starting with `_` or `.`, which is exactly Next.js's
  `_next/static/...` asset directory. Omitting `all:` embeds the HTML
  (which references those assets) but not the assets themselves, so every
  JS/CSS chunk 404s at runtime while the page still "loads" with no
  visible error at build time.
