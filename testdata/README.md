# Testdata

Checked-in fixtures for unit and integration tests.

```text
testdata/
├── tests/
│   ├── valid/          # compile-clean YAML v1 documents
│   └── invalid/        # documents that must fail Compile with ErrConfig
├── environments/
│   └── valid/          # env files exercising {{var}} / ${ENV} resolution
├── openapi/
│   ├── valid/          # openapi.yaml + swagger.json (well-known names) that parse cleanly
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
