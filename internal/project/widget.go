package project

import (
	_ "embed"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const widgetScriptURL = "http://127.0.0.1:4488/widget.js"

//go:embed apilens-widget.tsx
var widgetClientSource string

var lastImport = regexp.MustCompile(`(?m)^import .+$`)

// injectAppWidget adds the live-hits overlay to the product app's root
// layout (Next.js) or index.html (Vite). Idempotent. App URLs are not
// changed.
func injectAppWidget(projectDir string, res *Result) error {
	if path := firstExisting(projectDir, []string{
		"app/layout.tsx", "app/layout.jsx", "app/layout.js",
		"src/app/layout.tsx", "src/app/layout.jsx", "src/app/layout.js",
	}); path != "" {
		return injectNextAppLayout(path, res)
	}
	if path := findRootAppLayout(projectDir); path != "" {
		return injectNextAppLayout(path, res)
	}
	if path := firstExisting(projectDir, []string{
		"pages/_document.tsx", "pages/_document.jsx", "pages/_document.js",
		"src/pages/_document.tsx", "src/pages/_document.jsx", "src/pages/_document.js",
	}); path != "" {
		return injectHTMLScript(path, res, true)
	}
	if path := firstExisting(projectDir, []string{"index.html", "public/index.html"}); path != "" {
		return injectHTMLScript(path, res, false)
	}
	res.Notes = append(res.Notes,
		`No app layout found to inject the overlay. Run init in the frontend repo (the Next/Vite app), or add: <script src="http://127.0.0.1:4488/widget.js" async></script>`)
	return nil
}

// findRootAppLayout walks for a Next.js root layout (app/layout.tsx),
// including monorepo paths like apps/web/src/app/layout.tsx. Nested
// route-group layouts are ignored.
func findRootAppLayout(root string) string {
	var found []string
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
		if entry.IsDir() {
			return nil
		}
		relSlash := filepath.ToSlash(rel)
		if strings.HasSuffix(relSlash, "/app/layout.tsx") ||
			strings.HasSuffix(relSlash, "/app/layout.jsx") ||
			strings.HasSuffix(relSlash, "/app/layout.js") ||
			relSlash == "app/layout.tsx" || relSlash == "app/layout.jsx" || relSlash == "app/layout.js" {
			if strings.Contains(relSlash, "/(") {
				return nil
			}
			found = append(found, path)
		}
		return nil
	})
	if len(found) == 0 {
		return ""
	}
	shortest := found[0]
	for _, p := range found[1:] {
		if len(p) < len(shortest) {
			shortest = p
		}
	}
	return shortest
}

func firstExisting(root string, rels []string) string {
	for _, rel := range rels {
		p := filepath.Join(root, rel)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func injectNextAppLayout(layoutPath string, res *Result) error {
	raw, err := os.ReadFile(layoutPath)
	if err != nil {
		return err
	}
	src := string(raw)
	already := strings.Contains(src, "ApiLensWidget") || strings.Contains(src, "apilens-widget")

	widgetPath := filepath.Join(filepath.Dir(layoutPath), "apilens-widget"+filepath.Ext(layoutPath))
	if filepath.Ext(layoutPath) == ".js" {
		widgetPath = filepath.Join(filepath.Dir(layoutPath), "apilens-widget.jsx")
	}
	if strings.HasSuffix(layoutPath, ".tsx") || strings.HasSuffix(layoutPath, ".jsx") || strings.HasSuffix(layoutPath, ".js") {
		if err := os.WriteFile(widgetPath, []byte(widgetClientSource), 0o644); err != nil {
			return err
		}
		if already {
			res.Created = append(res.Created, widgetPath+" (overlay refreshed)")
		} else {
			res.Created = append(res.Created, widgetPath)
		}
	}

	if already {
		res.Skipped = append(res.Skipped, layoutPath+" (widget already present)")
		return nil
	}

	src = addImport(src, `import ApiLensWidget from "./apilens-widget";`)
	src = insertWidgetJSX(src)
	if err := os.WriteFile(layoutPath, []byte(src), 0o644); err != nil {
		return err
	}
	res.Created = append(res.Created, layoutPath+" (live-hits overlay)")
	return nil
}

func addImport(src, line string) string {
	if strings.Contains(src, `from "./apilens-widget"`) {
		return src
	}
	locs := lastImport.FindAllStringIndex(src, -1)
	if len(locs) == 0 {
		if strings.HasPrefix(strings.TrimSpace(src), `"use client"`) || strings.HasPrefix(strings.TrimSpace(src), `'use client'`) {
			nl := strings.Index(src, "\n")
			if nl >= 0 {
				return src[:nl+1] + line + "\n" + src[nl+1:]
			}
		}
		return line + "\n" + src
	}
	end := locs[len(locs)-1][1]
	return src[:end] + "\n" + line + src[end:]
}

func insertWidgetJSX(src string) string {
	if strings.Contains(src, "<ApiLensWidget") {
		return src
	}
	if i := strings.LastIndex(src, "</body>"); i >= 0 {
		return src[:i] + "        <ApiLensWidget />\n      " + src[i:]
	}
	for _, needle := range []string{"{children}", "{ children }"} {
		if i := strings.Index(src, needle); i >= 0 {
			return src[:i] + needle + "\n        <ApiLensWidget />" + src[i+len(needle):]
		}
	}
	return src + "\n"
}

func injectHTMLScript(path string, res *Result, jsx bool) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	src := string(raw)
	if strings.Contains(src, "widget.js") || strings.Contains(src, "apilens-widget") {
		res.Skipped = append(res.Skipped, path+" (widget already present)")
		return nil
	}
	tag := `<script src="` + widgetScriptURL + `" async></script>`
	if jsx {
		tag = `<script src="` + widgetScriptURL + `" async />`
	}
	if i := strings.LastIndex(src, "</body>"); i >= 0 {
		src = src[:i] + "        " + tag + "\n      " + src[i:]
	} else {
		src += "\n" + tag + "\n"
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		return err
	}
	res.Created = append(res.Created, path+" (live-hits overlay)")
	return nil
}
