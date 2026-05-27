package transform

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	var (
		mu       sync.Mutex
		changed  []string
		firstErr error
	)
	var wg sync.WaitGroup
	for _, path := range files {
		wg.Go(func() {
			c, perr := processFile(path, write)
			mu.Lock()
			defer mu.Unlock()
			if perr != nil && firstErr == nil {
				firstErr = perr
				return
			}
			if c {
				changed = append(changed, path)
			}
		})
	}
	wg.Wait()
	return changed, firstErr
}

func processFile(path string, write bool) (bool, error) {
	orig, readErr := os.ReadFile(path) //nolint:gosec // walked .go path under root, not user input
	if readErr != nil {
		return false, fmt.Errorf("read %s: %w", path, readErr)
	}
	if IsGenerated(orig) {
		return false, nil
	}
	out := Compact(StripComments(orig))
	if bytes.Equal(out, orig) {
		return false, nil
	}
	if !write {
		return true, nil
	}
	writeErr := os.WriteFile(path, out, 0o600) //nolint:gosec // walked .go path under root, not user input
	if writeErr != nil {
		return true, fmt.Errorf("write %s: %w", path, writeErr)
	}
	return true, nil
}
func Apply(root string) ([]string, error) { return run(root, true) }
func Check(root string) ([]string, error) { return run(root, false) }
