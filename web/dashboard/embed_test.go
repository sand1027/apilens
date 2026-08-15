package dashboard

import (
	"io/fs"
	"testing"
)

// TestFS_IncludesNextStaticAssets guards against the go:embed "_"-prefix
// exclusion bug this package hit once already: without the "all:" prefix
// on the go:embed directive, every file under "_next/" (Next.js's asset
// directory) is silently dropped, and every JS/CSS chunk 404s at runtime
// while the build itself reports success. This test fails loudly if that
// regresses.
func TestFS_IncludesNextStaticAssets(t *testing.T) {
	f := FS()

	if _, err := fs.Stat(f, "index.html"); err != nil {
		t.Fatalf("expected index.html in the embedded FS: %v", err)
	}

	found := false
	err := fs.WalkDir(f, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && len(path) > 6 && path[:6] == "_next/" {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir: %v", err)
	}
	if !found {
		t.Error("expected at least one file under _next/ in the embedded FS — " +
			"if this fails, check embed.go uses \"go:embed all:frontend/out\" (with the all: prefix)")
	}
}
