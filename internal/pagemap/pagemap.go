// Package pagemap implements plan.md v8's "Page-to-API mapping" —
// grouping captured API calls by the page that triggered them. The only
// signal watch has for this is the Referer header the browser sends on
// each XHR/fetch, which docs/11-risks-and-gaps.md G19 explicitly flags as
// unreliable ("Do not fake it from Referer in MVP"): a Referer can be
// absent (Referrer-Policy: no-referrer, some fetch() calls), stale (an
// SPA that never reloads the page), or simply not sent by some clients.
//
// v8 ships this anyway per plan.md's explicit ask, but keeps G19's
// honesty: every mapping is presented as an inferred/best-effort signal
// derived from Referer, not as ground truth "this page calls this API".
// Callers (CLI/dashboard output) must not present it as authoritative.
package pagemap

import (
	"net/http"
	"net/url"
	"sort"

	"github.com/sandeepv/apilens/internal/domain"
)

// APICall is one endpoint identity observed for a page.
type APICall struct {
	Method domain.Method
	Path   string // canonical (domain.NormalizePath) form
	Count  int    // number of times this page->call pair was observed
}

// PageMapping is every distinct API call observed with a given page as
// its Referer.
type PageMapping struct {
	Page  string // the referer URL's path (query stripped)
	Calls []APICall
}

// Build groups exchanges by their captured Referer header's path and
// counts distinct (method, path) calls per page. Exchanges with no
// Referer are grouped under the reserved page key "" — callers should
// treat that bucket separately (it means "no page signal available", not
// "the page whose path happens to be empty").
func Build(exchanges []domain.Exchange) []PageMapping {
	type key struct {
		page   string
		method domain.Method
		path   string
	}
	counts := map[key]int{}

	for _, ex := range exchanges {
		page := refererPath(ex.Request.Headers)
		callPath := requestPath(ex.Request.URL)
		k := key{page: page, method: ex.Request.Method, path: domain.NormalizePath(callPath)}
		counts[k]++
	}

	byPage := map[string][]APICall{}
	for k, count := range counts {
		byPage[k.page] = append(byPage[k.page], APICall{Method: k.method, Path: k.path, Count: count})
	}

	var pages []string
	for p := range byPage {
		pages = append(pages, p)
	}
	sort.Strings(pages)

	out := make([]PageMapping, 0, len(pages))
	for _, p := range pages {
		calls := byPage[p]
		sort.Slice(calls, func(i, j int) bool {
			if calls[i].Path != calls[j].Path {
				return calls[i].Path < calls[j].Path
			}
			return calls[i].Method < calls[j].Method
		})
		out = append(out, PageMapping{Page: p, Calls: calls})
	}
	return out
}

func refererPath(h http.Header) string {
	if h == nil {
		return ""
	}
	ref := h.Get("Referer")
	if ref == "" {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ref // unparseable but non-empty — still a distinct signal
	}
	if u.Path == "" {
		return "/"
	}
	return u.Path
}

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
