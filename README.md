# ApiLens

Developer-focused API discovery, inspection, replay, and automated testing — written in Go.

The CLI is the primary interface. A web dashboard comes later and will use the **same Go core engine**. ApiLens is not a Postman clone: the differentiator is automatic API discovery and runtime monitoring.

## Status

**v1 (Runner) through v5 (CI) have shipped.** You can `apilens init`,
write YAML tests, `apilens run` them, `apilens discover` OpenAPI specs or
Express routes into a registry, `apilens watch` a local proxy to see the
APIs your app actually calls, turn a captured call into a saved test with
`apilens generate`, and wire the whole thing into CI with `--format junit`
and `--quiet` — see [.github/workflows/apilens-example.yml](.github/workflows/apilens-example.yml).

- Product releases: [plan.md](plan.md)
- Architecture pack: [docs/README.md](docs/README.md)
- Diagrams: [architecture/](architecture/)
- Changelog: [CHANGELOG.md](CHANGELOG.md)

## Try it

```bash
go build -o apilens ./cmd/apilens
./apilens init
# edit .apilens/environments/local.yaml and .apilens/tests/**/*.yaml
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

## Intended workflow

```
Application → Discover → Registry → Watch → Capture → Inspect → Replay → Generate → Test → Report → CI
```

## CLI

Shipped (v1 through v5):

```text
apilens init
apilens version
apilens env list|use|show
apilens run [--filter --method --tag --sequential --parallel --fail-fast --format --quiet]
apilens test <ref> [--method]
apilens discover [--source] [--path] [--verbose]
apilens list [--method] [--tag]
apilens inspect <ref> [--method] [--live]
apilens watch [--bind] [--port] [--allow-remote] [--upstream] [--filter] [--host] [--all]
apilens history list [--limit] / apilens history show <id> [--verbose]
apilens replay <id> [--method] [--url] [--header] [--unset] [--query]
apilens generate <id> [--out] [--force]
```

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

Planned for v6+ (not yet implemented):

```text
apilens ui
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
