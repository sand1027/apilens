# ApiLens Architecture Pack

This folder is the source of truth until implementation starts.

**Do not write application code until this pack is reviewed.** The goal of the review is to lock:

- the core engine boundary
- plugin contracts
- CLI and test DSL
- security defaults
- Phase 1 implementation order

## Reading order

1. [01 — Architecture](01-architecture.md) — system shape, layers, flows
2. [02 — Packages](02-packages.md) — Go module boundaries
3. [03 — Plugins](03-plugins.md) — how discovery, reporters, assertions, and auth extend
4. [04 — Interfaces](04-interfaces.md) — contracts between modules
5. [05 — CLI](05-cli.md) — command surface
6. [06 — Test DSL](06-test-dsl.md) — YAML tests and assertions
7. [07 — Discovery](07-discovery.md) — finding APIs
8. [08 — Proxy](08-proxy.md) — runtime capture, history, replay
9. [09 — Security](09-security.md) — redaction and bind rules
10. [10 — Plan](10-plan.md) — v1 engineering PR order
11. [Product plan](../plan.md) — v1 to v10 releases
12. [11 — Risks and gaps](11-risks-and-gaps.md) — what can go wrong, what the spec left open
13. [12 — Decisions](12-decisions.md) — architectural decision records

## Non-negotiables

1. CLI first. Web UI later. Same engine.
2. Discovery is a first-class feature, not an import wizard.
3. Runtime monitoring is the differentiator — design it now, implement it after the core runner is stable.
4. No duplicated test/assertion/HTTP logic between CLI and UI.
5. Secrets are masked and not persisted by default.
6. Prefer simple Go and compile-time plugins over premature abstraction.
7. Keep framework-specific discovery isolated behind a provider interface.

## Review checklist

- [ ] Hexagonal core + adapter surfaces is accepted
- [ ] Compile-time plugin model is accepted
- [ ] `pkg/apilens` is the only public Go surface
- [ ] Test DSL v1 is accepted
- [ ] Inspect / test / run semantics are accepted
- [ ] Watch + in-memory history constraints are accepted
- [ ] Security defaults are accepted
- [ ] Phase 1 cut line is accepted
- [ ] Open gaps in [11-risks-and-gaps.md](11-risks-and-gaps.md) are decided or deferred
