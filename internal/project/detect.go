package project

import (
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// detected is what Init infers from the tree so it does not write a REST
// health check into a GraphQL repo, and does not invent a frontend URL.
type detected struct {
	GraphQL bool
	OpenAPI bool
	BaseURL string
}

var (
	graphqlURLKey = regexp.MustCompile(`(?m)^(?:export\s+)?[A-Za-z0-9_]*GRAPHQL[A-Za-z0-9_]*\s*=\s*["']?(https?://[^"'#\s]+)`)
	apiURLKey     = regexp.MustCompile(`(?m)^(?:export\s+)?(?:API_URL|BASE_URL|SERVER_URL)\s*=\s*["']?(https?://[^"'#\s]+)`)
)

var envFileNames = []string{".env", ".env.local", ".env.development", ".env.development.local"}

var wellKnownGraphQL = []string{"schema.graphql", "schema.gql", "schema.graphqls"}
var wellKnownOpenAPI = []string{"openapi.yaml", "openapi.yml", "openapi.json", "swagger.yaml", "swagger.yml", "swagger.json"}

func detectProject(root string) detected {
	d := detected{BaseURL: "http://localhost:5000"}
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		if rel != "." && shouldSkipDetectDir(entry, rel) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(entry.Name())
		if entry.IsDir() {
			return nil
		}
		for _, n := range wellKnownGraphQL {
			if name == n {
				d.GraphQL = true
			}
		}
		if strings.HasSuffix(name, ".graphql") || strings.HasSuffix(name, ".gql") || strings.HasSuffix(name, ".graphqls") {
			d.GraphQL = true
		}
		for _, n := range wellKnownOpenAPI {
			if name == n {
				d.OpenAPI = true
			}
		}
		for _, envName := range envFileNames {
			if entry.Name() == envName {
				if u := baseURLFromEnvFile(path); u != "" && d.BaseURL == "http://localhost:5000" {
					d.BaseURL = u
				}
			}
		}
		return nil
	})
	if d.GraphQL && d.BaseURL == "http://localhost:5000" {
		d.BaseURL = "http://localhost:3000"
	}
	return d
}

func shouldSkipDetectDir(entry os.DirEntry, rel string) bool {
	if !entry.IsDir() {
		return false
	}
	name := entry.Name()
	switch name {
	case "node_modules", "vendor", ".git", "dist", "build", ".next", ".apilens", "coverage":
		return true
	}
	depth := strings.Count(rel, string(os.PathSeparator)) + 1
	return depth > 4
}

func baseURLFromEnvFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := string(raw)
	if m := graphqlURLKey.FindStringSubmatch(text); len(m) == 2 {
		return originOf(m[1])
	}
	if m := apiURLKey.FindStringSubmatch(text); len(m) == 2 {
		return originOf(m[1])
	}
	return ""
}

func originOf(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}
