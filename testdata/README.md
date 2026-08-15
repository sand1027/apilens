# Testdata

Checked-in fixtures for unit and integration tests.

```text
testdata/
├── tests/
│   ├── valid/          # compile-clean YAML v1 documents
│   │   ├── contract-assertions.yaml  # v7: json.schema + json.matches + json.length together
│   │   ├── schema-file.yaml           # v7: json.schema_file loading user.schema.json
│   │   └── user.schema.json           # shared JSON Schema used by schema-file.yaml
│   └── invalid/        # documents that must fail Compile with ErrConfig
│       ├── bad-json-schema.yaml       # v7: malformed JSON Schema — config error, not a runtime failure
│       ├── bad-regex.yaml             # v7: invalid regex passed to json.matches
│       └── both-schema-forms.yaml     # v7: schema + schema_file both set is a config error
├── environments/
│   └── valid/          # env files exercising {{var}} / ${ENV} resolution
├── openapi/
│   ├── valid/          # openapi.yaml + swagger.json (well-known names) that parse cleanly
│   │   └── contract-spec.yaml         # v7: response schemas for internal/contract fixture tests
│   └── invalid/        # a structurally broken spec (skip, don't abort)
├── express/
│   ├── simple/         # package.json + app.js: app.get/post, a dynamic
│   │                     route call that must be skipped, not guessed
│   └── mounted/        # express.Router() + app.use('/api/users', router)
│                         mount-prefix resolution
└── captures/           # v3 — redacted exchange samples (not yet added)
```

Do not put secrets in this tree. `testdata/environments/valid/local.yaml`
references `${AUTH_TOKEN}` — tests that load it must set that env var
themselves (see internal/environment tests).
