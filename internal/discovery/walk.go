package discovery

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// DefaultIgnoreDirs mirrors the ignore list every walk-based provider uses
// (docs/11-risks-and-gaps.md R12, docs/07-discovery.md section 3/4). It
// covers both Node and Go project layouts so new framework providers
// don't each redeclare it.
var DefaultIgnoreDirs = map[string]bool{
	"node_modules": true, "vendor": true, ".git": true,
	"dist": true, "build": true, ".apilens": true,
}

// WalkSourceFiles walks root depth-limited (relative to root, 1-indexed),
// skipping ignoreDirs, and collects files whose extension (including the
// leading dot, e.g. ".go") is in extensions. Shared by multiple framework
// providers so each does not need to hand-roll its own directory walk
// (docs/03-plugins.md section 4: "Shared helpers go in discovery or
// domain" — providers must not import each other directly).
func WalkSourceFiles(root fs.FS, extensions []string, ignoreDirs map[string]bool, maxDepth int) ([]string, error) {
	extSet := make(map[string]bool, len(extensions))
	for _, e := range extensions {
		extSet[e] = true
	}
	var results []string
	err := fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // unreadable entry: skip, don't abort the walk
		}
		if p == "." {
			return nil
		}
		depth := strings.Count(p, "/") + 1
		if d.IsDir() {
			if ignoreDirs[d.Name()] || depth > maxDepth {
				return fs.SkipDir
			}
			return nil
		}
		if depth > maxDepth {
			return nil
		}
		if extSet[path.Ext(p)] {
			results = append(results, p)
		}
		return nil
	})
	sort.Strings(results)
	return results, err
}

// JoinURLPath joins a mount/group prefix with a route suffix, always
// producing exactly one "/" between them and collapsing a trailing "/"
// back down (so prefix "/api/users" + suffix "/" == "/api/users", not
// "/api/users/"). Shared by every framework provider that resolves a
// mount/group prefix onto a route (Express's own joinPath predates this
// and is left as-is to avoid touching working code; new providers should
// call this one).
func JoinURLPath(prefix, suffix string) string {
	var p string
	if prefix == "" {
		p = suffix
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	} else {
		p = strings.TrimSuffix(prefix, "/") + "/" + strings.TrimPrefix(suffix, "/")
		if !strings.HasPrefix(p, "/") {
			p = "/" + p
		}
	}
	if len(p) > 1 && strings.HasSuffix(p, "/") {
		p = strings.TrimSuffix(p, "/")
	}
	return p
}
