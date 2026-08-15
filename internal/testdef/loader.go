package testdef

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sandeepv/apilens/internal/domain"
)

// Loader implements docs/04-interfaces.md section 6.
type Loader struct{}

// NewLoader builds a Loader.
func NewLoader() *Loader { return &Loader{} }

// LoadAll walks root recursively for "*.yaml" / "*.yml" files and compiles
// each into a TestCase. Hidden files and "*.secrets.yaml" are skipped
// (docs/04-interfaces.md section 6). Results are sorted by file path for
// deterministic run order.
func (l *Loader) LoadAll(root string) ([]domain.TestCase, error) {
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, domain.NewConfigError(fmt.Sprintf("reading tests directory %s", root), err)
	}
	if !info.IsDir() {
		tc, err := l.LoadFile(root)
		if err != nil {
			return nil, err
		}
		return []domain.TestCase{tc}, nil
	}

	var paths []string
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			return nil
		}
		if strings.Contains(name, ".secrets.") {
			return nil
		}
		if strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") {
			paths = append(paths, path)
		}
		return nil
	})
	if err != nil {
		return nil, domain.NewConfigError(fmt.Sprintf("walking tests directory %s", root), err)
	}
	sort.Strings(paths)

	tests := make([]domain.TestCase, 0, len(paths))
	for _, p := range paths {
		tc, err := l.LoadFile(p)
		if err != nil {
			return nil, err
		}
		tests = append(tests, tc)
	}
	return tests, nil
}

// LoadFile compiles a single YAML file into a TestCase.
func (l *Loader) LoadFile(path string) (domain.TestCase, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return domain.TestCase{}, domain.NewConfigError(fmt.Sprintf("reading test file %s", path), err)
	}
	return Compile(raw, path)
}
