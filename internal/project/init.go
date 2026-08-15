// Package project creates and validates the ".apilens/" project layout
// described in docs/02-packages.md section 5 and docs/05-cli.md
// "apilens init".
package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

const configTemplate = `# ApiLens project configuration.
# See https://github.com/sandeepv/apilens/blob/main/docs/09-security.md
# for the security defaults below.

server:
  base_url: ""

testing:
  parallel: true
  retries: 1
  timeout: 10s
  reporter: terminal

security:
  capture_sensitive_headers: false
  sensitive_headers: []
  max_response_size: 5MB
  proxy_allow_remote: false

discovery:
  openapi:
    enabled: true
    paths: []
  graphql:
    enabled: true
    paths: []
  express:
    enabled: false
  fastify:
    enabled: false
  nestjs:
    enabled: false
  gin:
    enabled: false
  fiber:
    enabled: false
  echo:
    enabled: false
  ignore: []
  tags: {}

watch:
  bind: 127.0.0.1
  port: 8888
  path_prefix: ""
  ignore_extensions: [js, css, map, png, jpg, svg, woff2]

# db.connections is opt-in (empty by default).
# db:
#   connections:
#     main:
#       driver: sqlite
#       dsn: "${DATABASE_URL}"
`

const gitignoreSnippet = `
# ApiLens project artifacts
.apilens/reports/
.apilens/history/
.apilens/.current-env
.apilens/environments/*.secrets.yaml
`

const restHealthTemplate = `version: 1
name: Health
description: Sanity check that the API is reachable
tags: [smoke]

request:
  method: GET
  url: "{{base_url}}/health"

assert:
  status:
    equals: 200
`

const graphqlSmokeTemplate = `version: 1
name: GraphQL reachable
description: |
  Does not change frontend or backend URLs. Hits {{base_url}}/graphql.
  Query.__typename is always present; HTTP 200 means the server is up.
tags: [smoke, graphql]

request:
  graphql:
    query: |
      query { __typename }

assert:
  status:
    equals: 200
`

// Result reports what Init did, for the CLI to print.
type Result struct {
	Created  []string
	Skipped  []string
	Detected string
	Notes    []string
}

// Init creates the .apilens/ tree rooted at projectDir. Existing files are
// left untouched unless force is true, in which case config.yaml is
// overwritten (docs/05-cli.md: "Refuses to overwrite config.yaml unless
// --force").
func Init(projectDir string, force bool) (Result, error) {
	var res Result
	det := detectProject(projectDir)
	res.Detected = kindOf(det)
	res.Notes = initNotes(det)

	root := filepath.Join(projectDir, ".apilens")

	dirs := []string{
		root,
		filepath.Join(root, "tests"),
		filepath.Join(root, "environments"),
		filepath.Join(root, "api"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return res, domain.NewConfigError(fmt.Sprintf("creating directory %s", d), err)
		}
	}

	localEnv := fmt.Sprintf(`# Local environment. Do not put secrets in this file —
# use ${ENV_VAR} so they stay in the process environment (not git).
#
# Frontend and backend URLs stay as they are. apilens watch is a forward
# proxy on :8888; use "apilens watch --browser" so localhost traffic is
# captured without changing NEXT_PUBLIC_* or the API listen port.
base_url: %s
variables:
  token: "${AUTH_TOKEN}"
`, det.BaseURL)

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(root, "config.yaml"), configTemplate},
		{filepath.Join(root, "tests", ".gitkeep"), ""},
		{filepath.Join(root, "environments", "local.yaml"), localEnv},
		{filepath.Join(root, "api", ".gitkeep"), ""},
	}

	for _, f := range files {
		overwriteAllowed := force && filepath.Base(f.path) == "config.yaml"
		existed := fileExists(f.path)
		if existed && !overwriteAllowed {
			res.Skipped = append(res.Skipped, f.path)
			continue
		}
		if err := os.WriteFile(f.path, []byte(f.content), 0o644); err != nil {
			return res, domain.NewConfigError(fmt.Sprintf("writing %s", f.path), err)
		}
		res.Created = append(res.Created, f.path)
	}

	smokeRel := "tests/smoke/health.yaml"
	smokeBody := restHealthTemplate
	if det.GraphQL {
		smokeRel = "tests/smoke/graphql.yaml"
		smokeBody = graphqlSmokeTemplate
	}
	smokeTestPath := filepath.Join(root, smokeRel)
	if !fileExists(smokeTestPath) {
		if err := os.MkdirAll(filepath.Dir(smokeTestPath), 0o755); err != nil {
			return res, domain.NewConfigError("creating tests/smoke directory", err)
		}
		if err := os.WriteFile(smokeTestPath, []byte(smokeBody), 0o644); err != nil {
			return res, domain.NewConfigError("writing sample test", err)
		}
		res.Created = append(res.Created, smokeTestPath)
	} else {
		res.Skipped = append(res.Skipped, smokeTestPath)
	}

	if err := appendGitignore(projectDir); err != nil {
		return res, err
	}

	if err := injectAppWidget(projectDir, &res); err != nil {
		return res, domain.NewConfigError("injecting live-hits overlay", err)
	}

	return res, nil
}

func kindOf(d detected) string {
	if d.GraphQL {
		return "graphql"
	}
	if d.OpenAPI {
		return "openapi"
	}
	return "http"
}

func initNotes(d detected) []string {
	notes := []string{
		"Do not change the frontend or backend URL for capture.",
		"apilens watch is a forward proxy on 127.0.0.1:8888.",
		`Capture with: apilens watch --browser`,
		"That opens Chrome with localhost proxied through ApiLens; the app still calls its normal URL.",
		`Secrets: export AUTH_TOKEN='...' then apilens run`,
		`Live hits overlay is in this app (blue chip, bottom-left). Keep apilens ui running.`,
		`Also on apilens ui, or apilens init --ui.`,
	}
	if d.GraphQL {
		notes = append([]string{
			"Detected GraphQL SDL. discovery.graphql is on.",
			"base_url is " + d.BaseURL + " (from the repo, or Apollo's local default).",
			"Then: apilens discover --source graphql",
		}, notes...)
	}
	return notes
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func appendGitignore(projectDir string) error {
	path := filepath.Join(projectDir, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return domain.NewConfigError("reading .gitignore", err)
	}
	if strings.Contains(string(existing), ".apilens/reports/") {
		return nil
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return domain.NewConfigError("opening .gitignore", err)
	}
	defer f.Close()
	if _, err := f.WriteString(gitignoreSnippet); err != nil {
		return domain.NewConfigError("writing .gitignore", err)
	}
	return nil
}
