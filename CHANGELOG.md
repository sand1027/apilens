# Changelog

All notable product releases will be listed here.

Format: version, date, summary. Details live in [plan.md](plan.md).

## Unreleased

`apilens run` and `apilens test <ref>` gained `--out <path>` (plus
`--out-format json|junit`, inferred from the path's extension when
omitted) to persist a report file on top of whatever prints to stdout —
independent knobs, so `--quiet` or `--format terminal` on screen doesn't
change what lands in the file. `.apilens/reports/` was already reserved
for this in `apilens init`'s `.gitignore`; it just had nothing writing to
it before now.

`apilens configure` is a new command that sets up the auth token and a
database connection without hand-editing YAML: `apilens configure --token
"$AUTH_TOKEN" --db-driver sqlite --db-dsn "file:./app.db"` (or run with no
flags for an interactive prompt). The token is written to
`.apilens/environments/<env>.secrets.yaml`; the database DSN is persisted
to a new gitignored `.apilens/.secrets.env` and referenced from
`config.yaml` as `${DATABASE_URL}`, never as a literal value. Both files
are loaded automatically on every subsequent command.

`assert.db` now also supports MongoDB (`db.connections.<name>.driver:
mongodb`), alongside sqlite/postgres. The YAML shape swaps `query`/`args`
for `collection`/`filter` (a plain filter document) plus an optional
`field` for `equals` checks (defaults to `_id`); `row_count_equals` and
`exists` work the same as the SQL shape. A connection is either SQL-shaped
or MongoDB-shaped, never both, and MongoDB DSNs must include a database
name in their path (`mongodb://host:27017/mydb`).

`apilens init` injects a live-hits chip into the product app (`app/layout.tsx`
or `index.html`). The chip is a React overlay (bottom-left), not a script
tag, so it shows even when `apilens ui` is down. With `apilens ui` on `:4488`
it polls live history. App URLs are not changed. `apilens init --ui` also
opens the dashboard. A second init refreshes the overlay component.

Live hits HUD also remains on `apilens ui`: a corner chip (proxy up/down,
last GraphQL op, status including `200*`, duration, hit count) expands to
a modal.

`<env>.secrets.yaml` now loads and merges over `<env>.yaml` (still
gitignored). A missing `AUTH_TOKEN` names that file in the error.
`apilens env show` still redacts secret keys. `apilens record` writes
GraphQL + bearer `{{token}}` through the same YAML marshal as generate.
Watch collapses duplicate GraphQL lines, surfaces history/append errors,
and tests bind an ephemeral port (`127.0.0.1:0`) instead of colliding on
`:8888`.

GraphQL-over-HTTP is first-class: `request.graphql` / `assert.graphql` in the
YAML DSL, SDL discovery (`discovery.graphql`, enabled by default), watch and
generate treat GraphQL POST bodies as `QUERY`/`MUTATION` operations instead of
a single `POST /graphql`, and HTTP 200 with `errors[]` fails
`graphql.no_errors`. Example suite: [examples/stance-graphql](examples/stance-graphql)
against HealthFlex Stance on `http://localhost:3000`.

Watch session history lives at `.apilens/history/session.jsonl` in the project
that started `watch`. `apilens ui` in another repo (Stance frontend vs API)
follows a pointer at `$TMPDIR/apilens-current-history` so the overlay and
dashboard show the same hits. `watch --browser` launches a dedicated
Chrome with `--proxy-server` and `--proxy-bypass-list=<-loopback>;…:4488` so
dashboard GraphQL to localhost is captured without changing app URLs. PAC is
not used: Chrome cannot send localhost through a PAC script.

## v1 — Runner (2026-08-15)

Shipped. `go.mod`, `cmd/apilens`, and the full v1 engine per
[docs/10-plan.md](docs/10-plan.md):

- `apilens init`, `version`, `env list|use|show`
- `apilens run`, `apilens test <ref>`
- Config (`.apilens/config.yaml`), environments, `{{var}}` and `${ENV}`
  interpolation (fail closed on missing variables)
- HTTP runner with per-request timeout and `max_response_size` cap
- YAML v1 test DSL, one test per file
- Assertion engine: `status`, `header`, `body`, `json` (dotted paths),
  `duration`
- Test runner: sequential and parallel execution, transport-only retries,
  `--fail-fast`
- Terminal and JSON reporters (JSON schema matches
  [docs/11-risks-and-gaps.md](docs/11-risks-and-gaps.md) G25)
- Exit codes `0` / `1` / `2`
- Security redactor on all display/report paths (headers and JSON body
  keys masked by default, even with `capture_sensitive_headers: true`)
- `examples/fixture-server` — a small fixture API plus a sample
  `.apilens` project with 4 passing tests

## v2 — Discover (2026-08-15)

Shipped. Discovery orchestrator, OpenAPI + Express providers, registry
cache, `discover`, `list`, `inspect` per [docs/07-discovery.md](docs/07-discovery.md):

- Discovery provider port + orchestrator with merge/dedupe (endpoint
  identity = normalized method + path; `openapi` source wins over
  `express` when both find the same route)
- `openapi` provider — OpenAPI 3 and Swagger 2 via `kin-openapi`
  (ADR-008), well-known filename lookup + depth-4 walk, a broken spec is
  skipped unless explicitly named with `--path`
- `express` provider — conservative regex route scan (`app.`/`router.`/
  named router variables only), `router.use` mount-prefix resolution,
  dynamic route calls are never guessed at (`SkippedDynamicCount`)
- `.apilens/api/registry.yaml` cache so `list`/`inspect` work in a new
  process (ADR-017)
- `apilens discover [--source] [--path] [--verbose]`
- `apilens list [--method] [--tag]`
- `apilens inspect <ref> [--method] [--live]` — spec-first, `--live`
  probes through the same shared HTTP runner (ADR-018)
- `discovery.openapi.{enabled,paths}` / `discovery.express.enabled` in
  `config.yaml`
- Fixed a latent bug along the way: environment loading no longer
  eagerly expands `${ENV}` secrets at load time, so `discover`/`list`/
  `inspect` don't require unrelated secrets (e.g. `AUTH_TOKEN`) to be set

## v3 — Watch (2026-08-15)

Shipped. Local forward proxy, in-memory history, session JSONL, `apilens
watch`/`history` per [docs/08-proxy.md](docs/08-proxy.md):

- Loopback-only forward proxy (`127.0.0.1:8888` default), bind policy
  enforced before the CLI reports "listening" (`--allow-remote` for the
  conscious unsafe override)
- HTTPS `CONNECT` is tunneled byte-for-byte, no MITM (ADR-020)
- `--upstream` reverse-proxy mode for apps that can't set `HTTP_PROXY`
- Redaction applied before any exchange is displayed, stored, or written
  to disk — same `internal/security` policy as `run`
- In-memory ring buffer (`history.max_entries`, default 1000) assigns
  session-scoped display IDs (`#1`, `#2`, ...)
- Session JSONL file (`$APILENS_HISTORY_FILE` or
  `/tmp/apilens-history-<hash>.jsonl`, mode `0600`) so a second terminal's
  `history`/(future `replay`/`generate`) can see what `watch` captured
- Static-asset noise filtering (`.js`/`.css`/`.png`/etc.) and
  `--filter`/`--host` path/host restriction, `--all` to disable
- Observed traffic upserts into the registry as `source: watch`,
  persisted to `.apilens/api/registry.yaml` on clean stop — never
  overwrites a richer OpenAPI/Express spec for the same endpoint
- `apilens watch [--bind] [--port] [--allow-remote] [--upstream]
  [--filter] [--host] [--all]`
- `apilens history list [--limit]` / `apilens history show <id>
  [--verbose]`
- Fixed a concurrency bug found during manual testing: the proxy's
  capture channel had two consumers racing on it; now a single internal
  pump assigns display IDs and republishes to the CLI on its own channel

## v4 — Replay (2026-08-15)

Shipped. `replay`, `generate`, path-matched `test` per
[docs/08-proxy.md](docs/08-proxy.md) sections 8-9:

- `internal/replay` — reconstructs a stored exchange's request and
  applies overrides (method, URL, header set/unset, query, body) before
  executing it through the same shared runner every other live call uses
- Replay always creates a **new** history entry; it never overwrites the
  one it replayed
- Refuses to replay a still-masked `Bearer ********`/`Basic ********`
  header — fails with a clear error instead of silently sending the
  literal asterisks; `--header` or environment auth unblocks it
- `internal/generate` — maps a captured exchange to a YAML v1 test:
  method, URL, safe headers, JSON body (dropped if it looks like a login
  payload), and a `status.equals` assertion on the captured status
- Generated tests never carry `Authorization`/`Cookie`/`Set-Cookie` or a
  sensitive-looking body — verified end-to-end that no captured secret
  ever reaches the generated YAML file
- Default output path `.apilens/tests/generated/<method>-<slug-path>.yaml`,
  refuses to overwrite without `--force`
- `apilens replay <id> [--method] [--url] [--header] [--unset] [--query]`
- `apilens generate <id> [--out] [--force]`
- Verified the full differentiator loop end-to-end with a real binary:
  `watch` → capture → `generate` → `run` → passing test

## v5 — CI (2026-08-15)

Shipped. JUnit reporter, `--quiet`, GitHub Actions example — v1's JSON and
exit codes packaged for machines, per plan.md v5:

- `internal/reporter.JUnitReporter` — standard
  `<testsuites><testsuite><testcase>` XML with `<failure>`/`<error>`/
  `<skipped>` sub-elements, registered as a third built-in reporter
  alongside terminal/json
- `apilens run --format junit` / `apilens test --format junit`
- `--quiet` (global flag, already declared in v1) now actually
  suppresses passing/skipped lines in the terminal reporter while still
  printing failures, errors, and the final summary — json/junit are
  unaffected since they already buffer and emit once
- `.github/workflows/apilens-example.yml` — a copy-paste GitHub Actions
  job; wired against the bundled `examples/fixture-server` so it's a
  real, runnable example rather than inert boilerplate
- No new secrets exposure: verified the JUnit output never contains
  `Authorization` or other redacted values, same policy as the JSON
  reporter
- Verified end-to-end with a real binary: `--format junit --quiet`
  produces valid, parseable XML with the correct exit code for both
  passing and failing suites

## v6 — Dashboard (2026-08-15)

Shipped. Local web dashboard, same engine, per plan.md v6:

- `internal/webapi` — JSON REST + SSE surface wrapping `pkg/apilens.Engine`
  exactly the way `internal/cli` does; every dashboard action is an
  existing Engine method (endpoints, discover, inspect, run, history,
  replay, generate, environments, watch start/stop/status/events)
- `apilens ui [--bind] [--port] [--allow-remote]` — same loopback-only
  bind policy as `watch` (docs/09-security.md section 5)
- Frontend: a Next.js (TypeScript, Tailwind) single-page app, statically
  exported and embedded into the `apilens` binary via `go:embed` — no
  separate Node process at runtime. Views: API Explorer, Request Builder
  (replay with overrides), History, Runtime Monitor (live SSE feed while
  `watch` runs), Test Runner, Environment switcher
- Multi-subscriber SSE broadcaster for the Runtime Monitor, so more than
  one open dashboard tab can watch the same live traffic
- Fixed two real bugs found during implementation:
  - `go:embed frontend/out` (without the `all:` prefix) silently drops
    any path starting with `_` — which is exactly Next.js's
    `_next/static/...` asset directory. Every JS/CSS chunk 404'd at
    runtime while the build and embed both reported success. Fixed to
    `go:embed all:frontend/out`, with a regression test
  - Several `react-hooks/set-state-in-effect` false positives on the
    ordinary fetch-on-mount pattern (a known, acknowledged upstream
    issue — facebook/react#34743, #34905)
- 22 new Go unit tests (`internal/webapi`) plus a Go-side embed
  regression test; `npm run lint` and `npm run build` both clean
- Verified end-to-end with a real built binary and the bundled fixture
  server: dashboard HTML + static assets load, discover/run/replay/
  generate/environments work through the REST API, and the Runtime
  Monitor's SSE stream delivers real proxied traffic live

## v7 — Contracts (2026-08-15)

Shipped. Tests can enforce response shape, not just status codes, and
captures/registry can become a reviewable OpenAPI spec, per plan.md v7:

- New assertion kinds, wired into `internal/assertions.Engine.Compile`
  alongside the existing status/header/body/json checks:
  - `json.<path>.schema` — inline JSON Schema, or `schema_file` (resolved
    relative to the test file's own directory) via
    `santhosh-tekuri/jsonschema/v6`
  - `json.<path>.matches` — regex against a string value
  - `json.<path>.length` — works on strings (char count), arrays, and
    objects (key count), not just arrays
  - A malformed schema or invalid regex is a **config error caught at
    Compile time**, before any HTTP call — never a surprise mid-suite
    (docs/06-test-dsl.md section 12's compile-vs-assertion-failure rule)
- `internal/specexport` — builds a real OpenAPI 3 document (via
  `kin-openapi`'s own types, so marshaling is spec-correct) from the
  registry plus the most recent captured exchange per endpoint: real
  status codes and inferred response schemas (object/array/string/
  integer-vs-number/boolean) come from captures when available
- `apilens spec export [--out] [--force] [--title] [--spec-version]` —
  refuses to overwrite an existing file without `--force`, same policy
  as `generate`
- `internal/contract` — loads a stored OpenAPI document and validates a
  captured or live-probed response against the response schema the spec
  declares for the status code actually returned (honors OpenAPI's `2XX`
  patterned fallback). An endpoint with no schema and no way to obtain a
  response is reported as **skipped**, never a false failure
- `apilens contract test --spec <path> [--live]` — `--live` probes
  through the same shared runner every other live call uses when no
  capture is available; terminal and `--format json` output, same exit
  code convention as `run` (`0` clean, `1` any failed/errored)
- 51 new unit tests across `internal/assertions`, `internal/testdef`,
  `internal/specexport`, and `internal/contract`; new testdata fixtures
  for schema/regex/length assertions (valid and intentionally-malformed)
  and a contract-testing OpenAPI fixture
- Verified end-to-end with a real built binary and the bundled fixture
  server: a schema+regex+length assertion test passes against live
  traffic, `spec export` writes a valid OpenAPI document, and
  `contract test --live` against a hand-written spec correctly passes an
  endpoint matching its schema while failing another whose response is
  missing a field the spec requires — exit code `1`, clear diagnostic

## v8 — Platforms (2026-08-15)

Shipped. Discovery works on more stacks; teams can mock and see (inferred)
API relationships, per plan.md v8:

- Five new discovery providers, each a conservative source-tree scan
  (never executes user code, never invents an endpoint not literally
  present in source):
  - `gin`, `fiber`, `echo` — Go source, detected via `go.mod`, resolving
    nested router-group prefixes (`r.Group("/api").Group("/users")`) the
    same way `express` resolves `router.use` mounts
  - `fastify` — JS/TS, detected via `package.json`, handles both the
    `.get('/path', ...)` call form and the `.route({ method, url })`
    object form
  - `nestjs` — TypeScript decorators (`@Controller('/prefix')` classes,
    `@Get()`/`@Post()`/etc methods), not call-expression scanning; a
    decorated method on a class without `@Controller()` is never treated
    as a route
  - All five are opt-in in `config.yaml` (`discovery.<name>.enabled`),
    same policy as `express` since day one
- `internal/discovery`: shared `WalkSourceFiles`/`JoinURLPath` helpers so
  the new providers don't duplicate directory-walk logic; extended the
  orchestrator's source-priority table
- Discover-time tag/ignore filters (`discovery.ignore` / `discovery.tags`
  in `config.yaml`): glob patterns that drop or tag endpoints the moment
  they're discovered, so `apilens list --tag` and generated tests inherit
  grouping without every test author repeating it by hand
- `apilens mock` — a local mock server (loopback-only, same bind policy
  as `watch`/`ui`) that answers from the registry: the most recent
  captured exchange for an endpoint is replayed verbatim when available,
  otherwise a minimal synthesized response (never invented field values)
- `apilens graph` — an inferred API dependency graph built from
  time-proximity clustering of captured traffic (`internal/graph`).
  Explicitly presented as a hint, not ground truth — ApiLens has no
  access to the application's real call graph
- `apilens history pagemap` — groups captured calls by the Referer header
  of the page that triggered them (`internal/pagemap`). Also explicitly a
  best-effort signal: Referer can be absent or stale, and this ships with
  that caveat in the CLI output itself, not just the docs
  (docs/11-risks-and-gaps.md G19)
- ~60 new unit tests across the 5 provider packages, `internal/discovery`
  (filters), `internal/graph`, `internal/mock`, and `internal/pagemap`
- Verified end-to-end with a real built binary: discovered a Gin fixture
  app with nested route groups, applied a discover-time ignore+tag
  config and confirmed both took effect, discovered Fiber/Echo/Fastify/
  NestJS fixtures each finding the expected routes, ran `apilens mock`
  against a real registry+capture (verified byte-for-byte replay of a
  captured response, 404 for unmatched routes, and a synthesized
  response where no capture existed), and produced a correct dependency
  graph and page map from real proxied `apilens watch` traffic

## v9 — Scale (2026-08-15)

Shipped. Tests can chain, ApiLens can generate more of them, and the same
runner can apply bounded load — per plan.md v9. AI-assisted generation was
scoped out of this release (no API key infrastructure to send content to
an external model, and plan.md requires it be strictly opt-in — better to
ship it deliberately later than half-build it now).

- **DSL v2 response chaining** — a test can set `id: <name>` under
  `version: 2` so a *later* test in the same suite can reference its
  response via `{{responses.<id>.status}}` / `.headers.<name>` /
  `.body[.<dotted.path>]`. Chaining syntax or an `id` field under
  `version: 1` (implicit or explicit) is a **config error caught at
  Compile time**, per docs/06-test-dsl.md section 11's explicit rule that
  chaining must be "a later DSL version, not a silent v1 add-on."
  `internal/environment.Resolver` gained a mutex-protected response
  store; `app.RunSuite` rejects duplicate test `id`s within a suite and
  **silently forces sequential execution** whenever any test in the
  filtered set uses chaining — chaining a response from test A into test
  B is fundamentally incompatible with unordered parallel execution
- **Load / soak mode** — `apilens run --load [--duration] [--iterations]
  [--workers]` repeats the matched tests across concurrent workers and
  reports latency percentiles (p50/p90/p95/p99) and error rate instead of
  pass/fail (`internal/loadtest`), reusing the exact same
  runner/environment/assertion stack a normal run uses. Chained tests are
  rejected from load mode (repeating a chained flow concurrently would
  race on the response store in ways the sequential-run guarantee
  doesn't cover)
- **Test recording sessions** — `apilens record` turns the current
  history (from `apilens watch`) into a chained DSL v2 suite on disk
  (`internal/recording`). Correlation between steps is strictly
  literal-value-match — a later step's URL segment or JSON body value is
  only rewritten into a `{{responses...}}` reference when it exactly
  matches a value actually seen in an earlier step's response, never a
  guess
- **Database assertions (opt-in plugin)** — `assert.db.<connection>`
  blocks (`query`, plus `row_count_equals` / `exists` / `equals`) let a
  test confirm an HTTP call actually persisted (or didn't persist) to a
  real database. Opt-in twice over: no connection exists until
  `db.connections.<name>` is configured in `config.yaml`, and every query
  is validated **read-only at Compile time** — any mutating SQL keyword
  is rejected before a connection even opens. Ships with sqlite
  (`modernc.org/sqlite`, pure Go, no CGO) and postgres (`jackc/pgx`)
  drivers; DSNs are credentials and are expanded from `${ENV}` lazily,
  same fail-closed discipline as environment file variables
- **Automatic test generation from OpenAPI examples** — `apilens spec
  generate --from <path>` reads a stored OpenAPI document (the same
  parse+validate path `contract test` already uses) and writes one YAML
  v1 test per operation that declares a usable example value. An
  operation with a required request body or path parameter but no
  example anywhere is skipped, not padded with an invented value
- ~110 new unit tests across `internal/environment`, `internal/testdef`,
  `internal/app`, `internal/loadtest`, `internal/recording`,
  `internal/dbassert`, `internal/assertions`, and `internal/openapigen`
- Verified end-to-end with real built binaries: a login→fetch-created-user
  chained suite actually chaining a freshly-created id through two real
  HTTP calls; `--load --iterations 200` and `--load --duration 1s`
  against a real server (42,635 real requests in one run, catching real
  errors under load); a real `apilens watch` → `apilens record` →
  `apilens run` loop turning genuine captured traffic into a passing
  chained suite; a real sqlite-backed fixture server where `db.main.
  exists: true` correctly passed after a real INSERT and failed for a
  nonexistent row; `apilens spec generate` producing 4 passing tests from
  a hand-written spec's examples against the fixture server

## v10 — Team

Not shipped. Remote runs, collaboration, cloud, GitHub/GitLab.
