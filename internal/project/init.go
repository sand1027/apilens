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
  # ignore: glob patterns (path.Match syntax) to drop from discover results
  # tags: maps a glob pattern to a tag name applied to matching endpoints
  ignore: []
  tags: {}

watch:
  bind: 127.0.0.1
  port: 8888
  path_prefix: ""
  ignore_extensions: [js, css, map, png, jpg, svg, woff2]
`

const localEnvTemplate = `# Local environment. Do not put real secrets in this file directly —
# use ${ENV_VAR} to pull them from the process environment at run time.
base_url: http://localhost:5000
variables:
  token: "${AUTH_TOKEN}"
`

const sampleTestTemplate = `version: 1
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

const gitignoreSnippet = `
# ApiLens project artifacts
.apilens/reports/
.apilens/history/
.apilens/.current-env
.apilens/environments/*.secrets.yaml
`

// Result reports what Init did, for the CLI to print.
type Result struct {
	Created []string
	Skipped []string
}

// Init creates the .apilens/ tree rooted at projectDir. Existing files are
// left untouched unless force is true, in which case config.yaml is
// overwritten (docs/05-cli.md: "Refuses to overwrite config.yaml unless
// --force").
func Init(projectDir string, force bool) (Result, error) {
	var res Result
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

	files := []struct {
		path    string
		content string
	}{
		{filepath.Join(root, "config.yaml"), configTemplate},
		{filepath.Join(root, "tests", ".gitkeep"), ""},
		{filepath.Join(root, "environments", "local.yaml"), localEnvTemplate},
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

	// A starter smoke test only on first init (not forced), so re-running
	// `init --force` doesn't clobber a user's edited health check.
	smokeTestPath := filepath.Join(root, "tests", "smoke", "health.yaml")
	if !fileExists(smokeTestPath) {
		if err := os.MkdirAll(filepath.Dir(smokeTestPath), 0o755); err != nil {
			return res, domain.NewConfigError("creating tests/smoke directory", err)
		}
		if err := os.WriteFile(smokeTestPath, []byte(sampleTestTemplate), 0o644); err != nil {
			return res, domain.NewConfigError("writing sample test", err)
		}
		res.Created = append(res.Created, smokeTestPath)
	} else {
		res.Skipped = append(res.Skipped, smokeTestPath)
	}

	if err := appendGitignore(projectDir); err != nil {
		return res, err
	}

	return res, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// appendGitignore adds the ApiLens ignore snippet to the project's
// .gitignore if it's not already present. Does not create a git repo
// (docs/02-packages.md section 5: "init ... does not create a git repo").
func appendGitignore(projectDir string) error {
	path := filepath.Join(projectDir, ".gitignore")
	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return domain.NewConfigError("reading .gitignore", err)
	}
	if strings.Contains(string(existing), ".apilens/reports/") {
		return nil // already present
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
