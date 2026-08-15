# 05 — CLI design

Binary name: `apilens`.

The CLI is the primary surface. It is a thin adapter over `pkg/apilens.Engine`.

Use Cobra + `spf13/pflag`. Global flags are persistent on the root command.

## 1. Command tree

```mermaid
flowchart TB
    Root[apilens]
	Root --> Init[init]
	Root --> UI[ui]
	Root --> Discover[discover]
    Root --> List[list]
    Root --> Inspect[inspect]
    Root --> Test[test]
    Root --> Run[run]
    Root --> Watch[watch]
    Root --> History[history]
    Root --> Replay[replay]
    Root --> Generate[generate]
    Root --> Env[env]
    Root --> Version[version]
    Env --> EnvList[list]
    Env --> EnvUse[use]
    Env --> EnvShow[show]
    History --> HistList[list]
    History --> HistShow[show]
```

`ui` is a local dashboard (`apilens ui`). It is not injected into the product app.

## 2. Global flags

| Flag | Env | Default | Purpose |
| --- | --- | --- | --- |
| `--project` | `APILENS_PROJECT` | `.` | Project root containing `.apilens/` |
| `--env` | `APILENS_ENV` | current / `local` | Environment name |
| `--config` | `APILENS_CONFIG` | `.apilens/config.yaml` | Config path |
| `--format` | `APILENS_FORMAT` | `terminal` | `terminal` or `json` |
| `--quiet` | | false | Errors only |
| `--verbose` | | false | Debug logs (still redacted) |

`--format json` applies to `run`, `test`, `list`, `discover`, `inspect`, and `history`. Human-only commands (`watch` live view, `init`) ignore it or print a machine event stream later.

## 3. Commands

### `apilens init`

Creates the `.apilens/` tree if missing. Refuses to overwrite `config.yaml` unless `--force`.

Detects GraphQL SDL (`schema.graphql` / `*.graphql`) and OpenAPI files. For GraphQL it writes `tests/smoke/graphql.yaml` and a `base_url` of `http://localhost:3000` (or the origin of `NEXT_PUBLIC_GRAPHQL_API_URL` / similar in `.env`). It does **not** rewrite frontend or backend URLs.

```text
.apilens/config.yaml
.apilens/tests/.gitkeep
.apilens/environments/local.yaml
.apilens/api/.gitkeep
```

`local.yaml` has `base_url` and `token: "${AUTH_TOKEN}"`. Capture without changing app URLs: `apilens watch --browser`.

Init injects a live-hits chip into `app/layout.tsx` (Next.js) or `index.html` when found. The chip renders in the product app even if `apilens ui` is down; with `apilens ui` on `:4488` it polls history. It does **not** rewrite frontend or backend URLs. A second `apilens init` refreshes the overlay component without rewriting the layout.

`apilens init --ui` also opens the local dashboard.

### `apilens ui`

Local dashboard (loopback only). Same Engine as the CLI.

```text
apilens ui
apilens ui --port 4488
apilens init --ui
```

The first thing on the page is a corner chip: proxy up/down, last status (HTTP + GraphQL `errors[]` as `200*`), last duration, session hit count. Click it for the live list.

### `apilens discover`

Runs enabled discovery providers and upserts the registry.

```text
apilens discover
apilens discover --source openapi
apilens discover --source graphql
apilens discover --path ./openapi.yaml
apilens discover --path ./schema.graphql
```

Terminal output:

```text
API DISCOVERY

GET     /api/users                 openapi
POST    /api/users                 openapi
GET     /api/users/:id             openapi

Found: 3 APIs
```

JSON: `{ "endpoints": [...], "count": 3 }`.

### `apilens list`

Reads the registry. If empty, prints a hint to run `discover`. Does not rediscover implicitly.

```text
apilens list
apilens list --method GET
apilens list --tag users
```

### `apilens inspect <ref>`

`ref` is a path (`/api/users`) or endpoint ID.

| Flag | Meaning |
| --- | --- |
| `--method` | Required when multiple methods share the path |
| `--live` | Execute a probe request through the runner |
| `--last` | Show the last matching history exchange if present |

Default (no `--live`): show registry spec — method, path, parameters, available schema, source.

This resolves a spec gap: inspect is useful before any traffic exists.

### `apilens test <ref>`

Runs tests that match `ref`.

`ref` may be:

- a URL path: `/api/users`
- a test file: `.apilens/tests/users/get-user.yaml`
- a test name: `Get User`

```text
apilens test /api/users
apilens test /api/users --method GET
apilens test .apilens/tests/users/get-user.yaml
```

If several tests match, all of them run (a filtered suite).

If none match: exit 2, tell the user to write a test or `apilens generate`.

Do **not** silently fire an unasserted probe. That would blur `inspect --live` and `test`.

### `apilens run`

Runs the full suite under `.apilens/tests`.

```text
apilens run
apilens run --env staging
apilens run --format json
apilens run --filter smoke
apilens run --parallel
apilens run --sequential
apilens run --fail-fast
```

`--filter` matches test name, tag, or path substring.

Default parallel/sequential comes from `testing.parallel` in config. Flags override.

Sample terminal output:

```text
API TEST RESULTS

✓ GET    /api/users             200    82ms
✓ GET    /api/users/1           200    91ms
✓ POST   /api/users             201   143ms
✗ DELETE /api/users/1           500   218ms
    status.equals: expected 204, got 500

Tests:    4
Passed:   3
Failed:   1
Success:  75%
```

### `apilens watch`

Starts the local **forward** proxy. Frontend and backend URLs stay unchanged. Browsers skip proxies for localhost. Capture with `--browser`: a dedicated Chrome using `--proxy-server` and `--proxy-bypass-list=<-loopback>` (PAC cannot proxy localhost in Chrome). Existing Safari/Chrome tabs will never appear in watch.

```text
apilens watch
apilens watch --browser --open http://localhost:3001
apilens watch --port 8888
```

`--upstream` is reverse-proxy mode and **does** require the client to call the proxy URL. Prefer `--browser`.

### `apilens history`

Session helper for watch/replay.

```text
apilens history list
apilens history show 42
```

Only meaningful while history exists. Watch writes `<project>/.apilens/history/session.jsonl` (gitignored, overridable with `$APILENS_HISTORY_FILE`) so a second terminal can `replay` / `generate` / `history list` while watch is running or after Ctrl+C. See [08-proxy.md](08-proxy.md).

### `apilens replay <id>`

```text
apilens replay 42
apilens replay 42 --method POST
apilens replay 42 --url http://localhost:5000/api/forms
apilens replay 42 --header "X-Debug: 1"
apilens replay 42 --unset Cookie
```

Replays through the runner. Prints a redacted inspect view. Does not write a test.

### `apilens generate <id>`

```text
apilens generate 42
apilens generate 42 --out .apilens/tests/forms/create-form.yaml
apilens generate 42 --force
```

Writes YAML with method, URL, safe headers, body (if non-sensitive), and `status.equals` on the captured status.

### `apilens env`

```text
apilens env list
apilens env use staging
apilens env show
apilens env show local
```

`use` writes `.apilens/.current-env` (gitignored). `--env` on any command overrides it for that invocation.

### `apilens version`

Prints version, commit, and build date. Needed for CI bug reports.

## 4. Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success. For `run`/`test`: all tests passed or skipped-only with no failures. |
| 1 | One or more tests failed or errored. |
| 2 | Usage, config, missing file, unknown id, security bind rejection, compile error. |

`discover` with zero endpoints is exit 0 plus a warning — empty can be valid. A broken OpenAPI file is exit 2.

## 5. Command → engine mapping

| Command | Engine method |
| --- | --- |
| `init` | `Init` |
| `ui` | Engine methods via the dashboard HTTP API |
| `discover` | `Discover` |
| `list` | `List` |
| `inspect` | `Inspect` |
| `test` / `run` | `Run` |
| `watch` | `Watch` |
| `replay` | `Replay` |
| `generate` | `Generate` |
| `env use` | `UseEnv` |
| `history` | `History` |

No command reaches `internal/runner` itself.

## 6. Help and tone

Short verbs. No ASCII art. No emoji in default output.

Secrets in verbose mode still pass through the redactor. `--verbose` is not a bypass.

## 7. Completion

Cobra completion (`apilens completion zsh`) is cheap and useful. Add it when the command tree is stable, not in the first skeleton PR.
