# fixture-server example

A tiny standalone HTTP API (`/health`, `/api/users`) plus a sample
`.apilens/` project that tests it. This is the v1 example referenced in
[plan.md](../../plan.md) ("v1 | Local fixture server + hand-written YAML
tests").

## Run it

From the repository root:

```bash
# Terminal 1 — start the fixture API
go run ./examples/fixture-server -addr :5050

# Terminal 2 — build apilens and run the sample suite
go build -o /tmp/apilens ./cmd/apilens
cd examples/fixture-server/apilens-project
AUTH_TOKEN=dev /tmp/apilens run
```

Expected output: 4 tests, all passing, exit code `0`.

```bash
AUTH_TOKEN=dev /tmp/apilens run --format json
echo $?
```

## Layout

```text
examples/fixture-server/
├── main.go                    # the fixture API itself
└── apilens-project/
    └── .apilens/
        ├── config.yaml
        ├── environments/local.yaml
        └── tests/
            ├── smoke/health.yaml
            └── users/
                ├── list-users.yaml
                ├── get-user.yaml
                └── create-user.yaml
```
