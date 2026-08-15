// Package dashboard embeds the built Next.js static export (plan.md v6:
// "web/dashboard/ (static or small frontend). Go serves it.") so the
// apilens binary is self-contained — no separate Node process, no
// internet fetch at runtime.
//
// The frontend source lives in web/dashboard/frontend (a Next.js app with
// `output: 'export'`). Running `npm run build` there produces
// web/dashboard/frontend/out/, which this file embeds via go:embed.
//
// out/ is checked in as build output would normally be gitignored, but
// since this is the actual asset the Go binary serves, it must be present
// for `go build` to succeed — see web/dashboard/frontend/README.md for
// the build step.
package dashboard

import (
	"embed"
	"io/fs"
)

// The "all:" prefix is required because go:embed otherwise silently
// excludes any file or directory whose name starts with "_" or "." —
// which is exactly Next.js's "_next/static/..." asset directory.
//
//go:embed all:frontend/out
var embedded embed.FS

// WidgetJS is the in-app overlay `apilens init` injects into the product
// app. Served at /widget.js from `apilens ui`.
//
//go:embed widget.js
var WidgetJS []byte

// FS returns the embedded static site rooted at its own top level (i.e.
// "index.html" not "frontend/out/index.html"), ready to hand to
// http.FileServer.
func FS() fs.FS {
	sub, err := fs.Sub(embedded, "frontend/out")
	if err != nil {
		// Only possible if the embed path above is wrong at compile time,
		// which would already fail the build — this is defensive, not a
		// realistic runtime path.
		panic(err)
	}
	return sub
}
