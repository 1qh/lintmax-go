package transform

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const goExt = ".go"

//nolint:gochecknoglobals // pkg-data: walk-skip set
var skipDirs = map[string]bool{".git": true, "vendor": true, "testdata": true, "node_modules": true}

func walkGo(root string) ([]string, error) {
	var out []string
	walk := func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && skipDirs[entry.Name()] {
			return filepath.SkipDir
		}
		if !entry.IsDir() && strings.HasSuffix(path, goExt) {
			out = append(out, path)
		}
		return nil
	}
	err := filepath.WalkDir(root, walk)
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", root, err)
	}
	return out, nil
}

func run(root string, write bool) ([]string, error) {
	files, err := walkGo(root)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, path := range files {
		orig, readErr := os.ReadFile(path) //nolint:gosec // walked .go path under root, not user input
		if readErr != nil {
			return nil, fmt.Errorf("read %s: %w", path, readErr)
		}
		if IsGenerated(orig) {
			continue
		}
		out := Compact(StripComments(orig))
		if bytes.Equal(out, orig) {
			continue
		}
		changed = append(changed, path)
		if write {
			writeErr := os.WriteFile(path, out, 0o600) //nolint:gosec // walked .go path under root, not user input
			if writeErr != nil {
				return nil, fmt.Errorf("write %s: %w", path, writeErr)
			}
		}
	}
	return changed, nil
}
func Apply(root string) ([]string, error) { return run(root, true) }
func Check(root string) ([]string, error) { return run(root, false) }
