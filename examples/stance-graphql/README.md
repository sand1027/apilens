# Stance GraphQL example

Hand-written ApiLens tests for HealthFlex Stance's Apollo API
(`POST /graphql` on `http://localhost:3000`). Operations and field names
match `stance_dashboard` SDL and `stance-dashboard-frontend/src/gql/queries.ts`
(`GET_CENTERS`, `LOGIN`, `appVersionInfo`, `ping`).

## Run the public suite

Stance API must already be up on port 3000.

From the ApiLens repo:

```bash
go build -o /tmp/apilens ./cmd/apilens
cd examples/stance-graphql
/tmp/apilens run --tag public
```

Expected: 3 tests pass (app version, centers, unauthenticated ping rejected).

`apilens run` without `--tag` also works: auth tests are `skip: true` so they
show as skipped, not failed.

## What each public test covers

| Test | Stance operation | Why it exists |
| --- | --- | --- |
| `app-version-info.yaml` | `appVersionInfo(platform: IOS)` | Public (`permissions.ts` allow). HTTP 200 **and** `errors[]` empty. |
| `centers.yaml` | `centers { _id name isOnline }` | Same query the dashboard uses (`GET_CENTERS`), trimmed to scalars. |
| `ping-unauthenticated.yaml` | `ping` without a JWT | Shield rejects this. HTTP 200 with `errors[]` — `graphql.no_errors: false`. |

Without GraphQL assertions, ping-without-auth would look like a passing REST
test (`status.equals: 200`). That is the bug these assertions close.

## Authenticated ping

1. Open Stance dashboard, sign in, copy the bearer token.
2. In `.apilens/tests/auth/ping.yaml` set `skip: false`.
3. Run:

```bash
AUTH_TOKEN='eyJ...' /tmp/apilens run --tag auth
```

## Login then ping (DSL v2 chaining)

1. Set `skip: false` on `login.yaml` and `ping-after-login.yaml`.
2. Run:

```bash
STANCE_EMAIL='you@healthflex.example' \
STANCE_PASSWORD='...' \
/tmp/apilens run --tag login
```

Do not put real passwords in YAML. `${STANCE_PASSWORD}` is expanded at run time.

## Discover operations from Stance SDL

The slim `schema.graphql` in this folder is enough to try discovery:

```bash
cd examples/stance-graphql
/tmp/apilens discover --source graphql
/tmp/apilens list --tag graphql
```

To index the **full** Stance schema, run discover from the Stance API repo
(it already has a merged `schema.graphql` at the root):

```bash
cd ~/Desktop/healthflex/stance_dashboard
/tmp/apilens init          # once, if this tree has no .apilens yet
/tmp/apilens discover --source graphql
/tmp/apilens list --method QUERY
```

`QUERY` / `MUTATION` in `list` are display methods. The runner always POSTs
to `/graphql`.

## Generate a test from a captured call

```bash
# terminal 1
cd examples/stance-graphql
/tmp/apilens watch --upstream http://localhost:3000

# terminal 2 — point the dashboard or curl at the proxy, then:
/tmp/apilens history list
/tmp/apilens generate <id>
```

Generated GraphQL tests get `request.graphql` plus `assert.graphql.no_errors`
/ `has_data`, not a raw JSON body.

## Layout

```text
examples/stance-graphql/
├── README.md
├── schema.graphql                 # slim SDL for `apilens discover`
└── .apilens/
    ├── config.yaml
    ├── environments/local.yaml    # base_url: http://localhost:3000
    └── tests/
        ├── public/                # no token — run these first
        └── auth/                  # skip: true until you set secrets
```
