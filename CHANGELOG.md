# Changelog

All notable product releases will be listed here.

Format: version, date, summary. Details live in [plan.md](plan.md).

## Unreleased

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

## v6 — Dashboard

Not shipped. Local web UI on the same engine.

## v7 — Contracts

Not shipped. Schema assertions, OpenAPI export, contract tests.

## v8 — Platforms

Not shipped. More frameworks, mocks, API graphs.

## v9 — Scale

Not shipped. Load, recording, DSL v2 chaining, optional AI generate.

## v10 — Team

Not shipped. Remote runs, collaboration, cloud, GitHub/GitLab.
