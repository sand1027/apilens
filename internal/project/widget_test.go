package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_InjectsNextAppLayoutWidget(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "app")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}
	layout := `import "./globals.css";

export default function RootLayout({ children }) {
  return (
    <html>
      <body>
        {children}
      </body>
    </html>
  );
}
`
	layoutPath := filepath.Join(appDir, "layout.tsx")
	if err := os.WriteFile(layoutPath, []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Init(dir, false)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	widget := filepath.Join(appDir, "apilens-widget.tsx")
	widgetSrc, err := os.ReadFile(widget)
	if err != nil {
		t.Fatalf("expected injected widget component: %v", err)
	}
	if !strings.Contains(string(widgetSrc), "createPortal") || !strings.Contains(string(widgetSrc), "Live hits") {
		t.Fatalf("widget is a script stub, expected in-app chip:\n%s", widgetSrc[:min(400, len(widgetSrc))])
	}
	if !strings.Contains(string(widgetSrc), "isOverlayPoll") {
		t.Fatal("widget must hide overlay polls of apilens ui")
	}
	got, err := os.ReadFile(layoutPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if !strings.Contains(s, `from "./apilens-widget"`) || !strings.Contains(s, "<ApiLensWidget />") {
		t.Fatalf("layout not injected:\n%s", s)
	}
	if !strings.Contains(s, "{children}") {
		t.Fatal("layout lost {children}")
	}

	res2, err := Init(dir, false)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	skipped := false
	for _, x := range res2.Skipped {
		if strings.Contains(x, "widget already present") {
			skipped = true
		}
	}
	if !skipped {
		t.Errorf("second init should skip widget, Skipped=%v Created=%v", res2.Skipped, res.Created)
	}

	stub := `"use client";
export default function ApiLensWidget() { return null; }
`
	if err := os.WriteFile(widget, []byte(stub), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dir, false); err != nil {
		t.Fatalf("third Init: %v", err)
	}
	refreshed, err := os.ReadFile(widget)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(refreshed), "createPortal") {
		t.Fatalf("expected init to refresh overlay component, got:\n%s", refreshed)
	}
}

func TestInit_InjectsNestedMonorepoLayout(t *testing.T) {
	dir := t.TempDir()
	layoutDir := filepath.Join(dir, "apps", "web", "src", "app")
	if err := os.MkdirAll(layoutDir, 0o755); err != nil {
		t.Fatal(err)
	}
	layout := `export default function RootLayout({ children }) {
  return <html><body>{children}</body></html>;
}
`
	if err := os.WriteFile(filepath.Join(layoutDir, "layout.tsx"), []byte(layout), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dir, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(layoutDir, "layout.tsx"))
	if !strings.Contains(string(got), "<ApiLensWidget />") {
		t.Fatalf("nested layout not injected:\n%s", got)
	}
}

func TestInit_InjectsIndexHTMLWidget(t *testing.T) {
	dir := t.TempDir()
	html := `<!doctype html><html><body><div id="root"></div></body></html>`
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(html), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(dir, false); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	if !strings.Contains(string(got), "127.0.0.1:4488/widget.js") {
		t.Fatalf("index.html not injected:\n%s", got)
	}
}
