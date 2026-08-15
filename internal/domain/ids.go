// Package domain holds the core types shared by every ApiLens package.
// Types only — no I/O, no YAML, no HTTP client, no CLI. See docs/02-packages.md.
package domain

import (
	"crypto/sha1"
	"encoding/hex"
	"regexp"
	"strings"
)

// EndpointID is a stable identifier for a discovered endpoint: a hash of
// normalized method + path (+ source in some contexts).
type EndpointID string

// ExchangeID is the internal UUID assigned to every HTTP exchange.
type ExchangeID string

// DisplayID is the monotonic, human-friendly, session-scoped counter shown
// as "#42" in watch/history/replay output.
type DisplayID int

// EnvName identifies an environment definition, e.g. "local" or "staging".
type EnvName string

// Method is an HTTP method, always stored upper-cased.
type Method string

// NormalizeMethod upper-cases and trims an HTTP method string.
func NormalizeMethod(m string) Method {
	return Method(strings.ToUpper(strings.TrimSpace(m)))
}

func (m Method) String() string { return string(m) }

// pathParamPattern matches Express/Gin-style (:id) and OpenAPI-style
// ({id}) path parameters so NormalizePath can canonicalize both to ":id"
// (docs/07-discovery.md section 2).
var pathParamPattern = regexp.MustCompile(`(:[a-zA-Z0-9_]+)|(\{[a-zA-Z0-9_]+\})`)

// NormalizePath implements the endpoint identity rules from
// docs/07-discovery.md section 2:
//   - strip trailing slash except "/"
//   - convert :id and {id} style params to a canonical ":id"
//   - drop query strings (identity is method+path only)
func NormalizePath(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		path = strings.TrimRight(path, "/")
	}
	if path == "" {
		path = "/"
	}
	return pathParamPattern.ReplaceAllString(path, ":id")
}

// NewEndpointID computes the stable identity hash from
// docs/07-discovery.md section 2: normalize(method) + normalize(path).
// Source is deliberately excluded — endpoints found by multiple providers
// must collapse to one ID so the orchestrator can merge their sources.
func NewEndpointID(method Method, path string) EndpointID {
	key := string(NormalizeMethod(string(method))) + " " + NormalizePath(path)
	sum := sha1.Sum([]byte(key))
	return EndpointID(hex.EncodeToString(sum[:]))
}
