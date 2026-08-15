package recording

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sandeepv/apilens/internal/domain"
	"github.com/sandeepv/apilens/internal/generate"
)

// WriteOptions configures WriteSuite.
type WriteOptions struct {
	Dir   string // output directory; files are named "01-<id>.yaml", "02-<id>.yaml", ...
	Force bool   // overwrite existing files
}

// WriteSuite writes each Step to its own numbered YAML v2 file in
// opts.Dir, preserving the recorded order in the filenames (the loader
// sorts by file path, so "01-..." must run before "02-..." — same
// determinism guarantee testdef.Loader.LoadAll already provides, just
// made explicit for a generated suite instead of relying on the author's
// own naming). GraphQL, auth, and header-dropping come from
// generate.MarshalYAML so `apilens record` cannot drift from
// `apilens generate`.
func WriteSuite(steps []Step, opts WriteOptions) ([]string, error) {
	if opts.Dir == "" {
		return nil, domain.NewConfigError("recording.WriteSuite requires an output directory", nil)
	}
	if err := os.MkdirAll(opts.Dir, 0o755); err != nil {
		return nil, domain.NewConfigError("creating recording output directory", err)
	}

	var written []string
	for i, step := range steps {
		filename := fmt.Sprintf("%02d-%s.yaml", i+1, step.ID)
		path := filepath.Join(opts.Dir, filename)
		if !opts.Force {
			if _, err := os.Stat(path); err == nil {
				return written, domain.NewConfigError(
					fmt.Sprintf("%s already exists — pass --force to overwrite", path), nil)
			}
		}
		content, err := generate.MarshalYAML(step.Test)
		if err != nil {
			return written, err
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return written, domain.NewConfigError("writing recorded test "+path, err)
		}
		written = append(written, path)
	}
	return written, nil
}
