# 12 — Architectural decision records

Status key: **Proposed** until this pack is reviewed, then **Accepted** or **Superseded**.

## ADR-001 — Hexagonal core, surfaces as adapters

**Status:** Proposed  
**Context:** CLI, future web UI, and CI must not diverge.  
**Decision:** All business logic lives behind `pkg/apilens.Engine`. CLI is Cobra over that facade.  
**Consequences:** Slightly more wiring up front. New surfaces stay thin.  
**Rejected:** Separate Node UI backend; copying HTTP code into handlers.

## ADR-002 — Compile-time plugins

**Status:** Proposed  
**Context:** Discovery, reporters, assertions, and auth will grow.  
**Decision:** Go interfaces + explicit registration in `cmd` / `internal/plugins`.  
**Consequences:** New providers require a rebuild. Acceptable for an in-tree CLI.  
**Rejected:** `plugin` `.so`; HashiCorp go-plugin for MVP.

## ADR-003 — YAML test DSL, not generated Go tests

**Status:** Proposed  
**Context:** QA and CI must edit tests without a Go toolchain.  
**Decision:** Versioned YAML documents compiled to `domain.TestCase`.  
**Consequences:** Less expressive than code. No hooks in v1.  
**Rejected:** Go test files as the primary DSL; JavaScript hooks.

## ADR-004 — In-memory history plus session JSONL

**Status:** Proposed  
**Context:** Spec says in-memory first; replay from another terminal is otherwise impossible.  
**Decision:** Ring buffer in process; redacted JSONL session file for watch.  
**Consequences:** IDs are session-scoped. No query language over history.  
**Rejected:** SQLite in MVP; durable product database.

## ADR-005 — Loopback-only proxy by default

**Status:** Proposed  
**Context:** A LAN-exposed intercepting proxy is a credential leak.  
**Decision:** Bind `127.0.0.1:8888`. Non-loopback requires `--allow-remote`.  
**Consequences:** Remote device capture needs an explicit unsafe flag.  
**Rejected:** Default `0.0.0.0`.

## ADR-006 — Redact by default

**Status:** Proposed  
**Context:** Captured traffic is full of tokens.  
**Decision:** `internal/security` is mandatory on display, persist, and generate. `capture_sensitive_headers` defaults false and still does not unmask reports.  
**Consequences:** Replay of captured auth requires env credentials.  
**Rejected:** Store raw and hope `.gitignore` is enough.

## ADR-007 — Cobra CLI

**Status:** Proposed  
**Context:** Nested commands (`env use`, `history show`) and completions.  
**Decision:** `spf13/cobra`.  
**Rejected:** Hand-rolled `os.Args`; urfave/cli (fine, but Cobra is the common Go CLI default).

## ADR-008 — OpenAPI via kin-openapi

**Status:** Proposed  
**Context:** Spec parsing is a graveyard of half-parsers.  
**Decision:** `getkin/kin-openapi` in Phase 2.  
**Consequences:** Dependency weight. Better than owning OAS semantics.

## ADR-009 — No web UI until Phase 5

**Status:** Proposed  
**Context:** UI will invent a second runner if built early.  
**Decision:** CLI + engine only through Phase 4.  
**Consequences:** Visual explorer waits. Core stays honest.

## ADR-010 — `pkg/apilens` is the only public surface

**Status:** Proposed  
**Context:** Leaking `internal/runner` makes the UI import internals.  
**Decision:** External code including a future UI package imports `pkg/apilens` only.  
**Consequences:** Facade must be designed with UI in mind now.

## ADR-011 — HTTP/1.1 first

**Status:** Proposed  
**Context:** HTTP/2 multiplex and HTTP/3 capture complicate the proxy.  
**Decision:** Stdlib HTTP client; HTTP/1.1 proxy.  
**Consequences:** Some modern stacks less visible in watch until later.

## ADR-012 — Dotted JSON paths in v1

**Status:** Proposed  
**Context:** Full JSONPath is a language.  
**Decision:** `data.items.0.name` only.  
**Consequences:** Limited queries. A later assertion plugin can add real JSONPath.

## ADR-013 — Integer display IDs and internal UUIDs

**Status:** Proposed  
**Context:** Humans want `replay 42`. Machines want stable IDs.  
**Decision:** Monotonic display ID per session + UUID on the exchange.  
**Consequences:** `#42` is not portable across sessions.

## ADR-014 — Explicit plugin wiring, no `init()` magic

**Status:** Proposed  
**Context:** `init()` registration hides dependencies and hurts tests.  
**Decision:** `main` (or `plugins.RegisterBuiltins`) lists every built-in.  
**Consequences:** One extra file to touch when adding a provider.

## ADR-015 — Fail closed on interpolation

**Status:** Proposed  
**Context:** Sending `{{token}}` or an empty secret looks like a successful request.  
**Decision:** Missing `{{var}}` or `${ENV}` is `ErrConfig` (exit 2).  
**Rejected:** Empty-string fallback.

## ADR-016 — One test per YAML file

**Status:** Proposed  
**Context:** Multi-doc YAML and `tests:` arrays complicate line errors.  
**Decision:** One document, one test. Suites are directories.  
**Consequences:** More files. Clearer generate/overwrite behavior.

## ADR-017 — Discovery writes a registry cache

**Status:** Proposed  
**Context:** `list` after `discover` must work in a new process.  
**Decision:** `.apilens/api/registry.yaml` is a replaceable cache.  
**Consequences:** Users may edit it; re-discover overwrites. Document that.

## ADR-018 — Inspect is spec-first

**Status:** Proposed  
**Context:** Spec text implies live response on every inspect.  
**Decision:** Spec from registry by default; `--live` / `--last` for traffic.  
**Consequences:** Matches discovery-first product; avoids surprise network calls.

## ADR-019 — No implicit probe on `test`

**Status:** Proposed  
**Context:** `apilens test /api/users` could mean "hit it and hope 200".  
**Decision:** Only configured YAML tests. Missing tests → exit 2.  
**Consequences:** Slightly stricter UX. Clearer meaning of "test".

## ADR-020 — Phase 3 ships without MITM

**Status:** Proposed  
**Context:** Local CA install is a support and security sink.  
**Decision:** Capture HTTP and optional reverse-proxy `--upstream`. MITM later.  
**Consequences:** HTTPS SPAs need a later flag or HTTP local APIs.

## ADR-021 — Stdlib HTTP, not a client SDK

**Status:** Proposed  
**Context:** Resty/Heimdall add behavior that is hard to reason about in a test runner.  
**Decision:** `net/http` with a small decorator for timeout and size.  
**Consequences:** We own retries at the testrunner layer only.

## ADR-022 — Reporters do not decide pass/fail

**Status:** Proposed  
**Context:** Easy to let the terminal reporter "compute" counts differently from JSON.  
**Decision:** `domain.Report` is computed by `testrunner`. Reporters format it.  
**Consequences:** All formats agree.

---

After review, flip each ADR to Accepted (or replace). Do not start PR1 with Proposed-but-verbally-rejected decisions.