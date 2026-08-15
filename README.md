# ApiLens

Developer-focused API discovery, inspection, replay, and automated testing — written in Go.

The CLI is the primary interface. A web dashboard comes later and will use the **same Go core engine**. ApiLens is not a Postman clone: the differentiator is automatic API discovery and runtime monitoring.

## Status

**v1 (Runner) through v9 (Scale) have shipped.** You can `apilens
init`, write YAML tests, `apilens run` them, `apilens discover` OpenAPI
specs or routes from Express/Fastify/NestJS/Gin/Fiber/Echo into a
registry, `apilens watch` a local proxy to see the APIs your app actually
calls, turn a captured call into a saved test with `apilens generate`,
wire the whole thing into CI with `--format junit` and `--quiet`, drive
all of it from a local browser dashboard with `apilens ui`, assert on
response shape with JSON Schema/regex/length checks, export the registry
to a real OpenAPI document with `apilens spec export`, check live or
captured responses against that document with `apilens contract test`,
serve mock responses with `apilens mock`, get an inferred API dependency
graph or page-to-API map from captured traffic, chain tests together with
DSL v2 response references, turn a whole watch session into a chained
suite with `apilens record`, apply bounded load with `apilens run --load`,
assert on real database state with opt-in `db.*` checks, and generate
tests straight from an OpenAPI spec's examples with `apilens spec
generate`.

- Product releases: [plan.md](plan.md)
- Architecture pack: [docs/README.md](docs/README.md)
- Diagrams: [architecture/](architecture/)
- Changelog: [CHANGELOG.md](CHANGELOG.md)

## Try it

```bash
go build -o apilens ./cmd/apilens
./apilens init
# next: apilens ui  — live hits in the corner widget
# or:  apilens init --ui
AUTH_TOKEN=dev ./apilens run
echo $?          # 0 or 1
```

Or run the bundled example against a small fixture API:

```bash
go run ./examples/fixture-server -addr :5050 &
cd examples/fixture-server/apilens-project
AUTH_TOKEN=dev go run ../../../cmd/apilens run
```

See [examples/fixture-server/README.md](examples/fixture-server/README.md).

GraphQL (HealthFlex Stance Apollo API on `:3000`):

```bash
go build -o /tmp/apilens ./cmd/apilens
cd examples/stance-graphql
/tmp/apilens run --tag public
```

See [examples/stance-graphql/README.md](examples/stance-graphql/README.md).

## Intended workflow

```
Application → Discover → Registry → Watch → Capture → Inspect → Replay → Generate → Test → Report → CI
```

## CLI

Shipped (v1 through v9):

```text
apilens init
apilens version
apilens env list|use|show
apilens run [--filter --method --tag --sequential --parallel --fail-fast --format --quiet]
apilens run --load [--duration] [--iterations] [--workers]
apilens test <ref> [--method]
apilens discover [--source] [--path] [--verbose]
apilens list [--method] [--tag]
apilens inspect <ref> [--method] [--live]
apilens watch [--bind] [--port] [--allow-remote] [--upstream] [--filter] [--host] [--all]
apilens history list [--limit] / apilens history show <id> [--verbose]
apilens history pagemap
apilens replay <id> [--method] [--url] [--header] [--unset] [--query]
apilens generate <id> [--out] [--force]
apilens record [--limit] [--out] [--force]
apilens ui [--bind] [--port] [--allow-remote]
apilens spec export [--out] [--force] [--title] [--spec-version]
apilens spec generate --from <path> [--out] [--force]
apilens contract test --spec <path> [--live]
apilens mock [--bind] [--port] [--allow-remote]
apilens graph [--window-ms]
```

Discovery now covers OpenAPI/Swagger, Express, Fastify, NestJS, Gin,
Fiber, and Echo. Every framework provider is opt-in
(`discovery.<name>.enabled: true` in `config.yaml`), same as Express since
v2. Discover-time filters group or drop endpoints as they're found:

```yaml
discovery:
  gin:
    enabled: true
  ignore:
    - /internal/*
  tags:
    "/api/*": users-suite
```

`apilens ui` starts a local web dashboard (localhost only) at
`http://127.0.0.1:4488`. A corner widget shows live hits, GraphQL
operation names, health (success vs `200*`), and p50/p95 timings. Click
it for the full list. Every action still calls the same Engine methods
the CLI does. See [web/dashboard/README.md](web/dashboard/README.md) for
the frontend build workflow.

`--format` accepts `terminal`, `json`, or `junit` on `run`/`test`. CI
example:

```bash
apilens run --format junit --quiet > junit.xml
```

Try the full watch → generate → run loop against the bundled fixture API:

```bash
go run ./examples/fixture-server -addr :5050 &
apilens watch --port 8888 &
curl -x http://127.0.0.1:8888 http://localhost:5050/health
# watch prints "#1  GET  http://localhost:5050/health  200  Nms"

apilens generate 1          # writes .apilens/tests/generated/get-health.yaml
apilens run                 # runs it, exit 0
```

Assert on response shape, export the registry to OpenAPI, and check it
against live traffic:

```bash
# In a test file's assert.json.<path> block:
#   schema: { type: object, required: [id, name] }   # or schema_file: ./x.schema.json
#   matches: '^[^@]+@[^@]+\.[^@]+$'
#   length: 3

apilens discover                              # populate the registry first
apilens spec export --out api/openapi.yaml    # OpenAPI 3 from registry + captures
apilens contract test --spec api/openapi.yaml --live   # probe + validate against it
```

Mock a downed API from what you've already discovered/captured, and get
a heuristic view of how your APIs relate:

```bash
apilens mock --port 4489       # serves captured examples, or a minimal synthesized response
apilens graph                  # inferred dependency graph from time-proximity in captured traffic
apilens history pagemap        # captured calls grouped by the page (Referer) that triggered them
```

Chain tests together (DSL v2), record a whole flow, apply load, and check
real database state after a call:

```yaml
# .apilens/tests/login.yaml
version: 2
id: login
name: Login
request:
  method: POST
  url: "{{base_url}}/api/login"
  body:
    json: { email: a@example.com, password: "{{password}}" }
assert:
  status: { equals: 200 }
---
# .apilens/tests/profile.yaml
version: 2
name: Get Profile
request:
  method: GET
  url: "{{base_url}}/api/me"
  headers:
    Authorization: "Bearer {{responses.login.body.token}}"
assert:
  status: { equals: 200 }
```

```bash
apilens watch --port 8888     # capture a real login-then-profile flow
apilens record                # turn it into a chained suite under .apilens/tests/recorded
apilens run                   # chained suites run sequentially, automatically

apilens run --load --duration 30s --workers 20   # bounded load pass with p50/p95/p99
```

```yaml
# config.yaml
db:
  connections:
    main: { driver: sqlite, dsn: "${DATABASE_URL}" }
```

```yaml
assert:
  status: { equals: 201 }
  db:
    main:
      query: "SELECT * FROM users WHERE email = 'a@example.com'"
      exists: true
```

Generate tests straight from an OpenAPI spec's own examples — no traffic
needed:

```bash
apilens spec generate --from api/openapi.yaml
```

Planned for v10+ (not yet implemented, and needs real infra decisions
before it can be built — hosting, auth, and secret storage for remote
workers/a cloud dashboard):

```text
apilens ci report --remote   # team collaboration, remote workers, cloud dashboard
```

## Documentation

| Doc | What it covers |
| --- | --- |
| [Architecture](docs/01-architecture.md) | Layers, flows, design patterns |
| [Packages](docs/02-packages.md) | Go module boundaries |
| [Plugins](docs/03-plugins.md) | Compile-time plugin host |
| [Interfaces](docs/04-interfaces.md) | Contracts between modules |
| [CLI](docs/05-cli.md) | Command structure |
| [Test DSL](docs/06-test-dsl.md) | YAML tests and assertions |
| [Discovery](docs/07-discovery.md) | OpenAPI and framework providers |
| [Proxy](docs/08-proxy.md) | Runtime capture and replay |
| [Security](docs/09-security.md) | Redaction and bind rules |
| [Product plan](plan.md) | v1–v10 releases |
| [MVP engineering plan](docs/10-plan.md) | v1 PR order |
| [Risks & gaps](docs/11-risks-and-gaps.md) | What to decide before coding |
| [Decisions](docs/12-decisions.md) | ADRs |
