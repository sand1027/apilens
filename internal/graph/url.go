package graph

import "net/url"

// requestPath extracts just the path portion of a captured request URL,
// mirroring internal/app's own requestPath helper (duplicated rather than
// imported to avoid an internal/graph -> internal/app dependency, which
// would invert the intended layering — app depends on graph, not the
// reverse).
func requestPath(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}
