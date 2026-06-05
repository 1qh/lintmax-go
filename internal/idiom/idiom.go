package idiom

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Issue struct {
	File string
	Name string
}

var gibberish = regexp.MustCompile(`\b(k[0-9a-f]{2,3}[A-Z]|ks[A-Z])[A-Za-z0-9]*\b`)

func Scan(_ context.Context, root string) ([]Issue, error) {
	seen := map[string]string{}
	walkErr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, rerr := os.ReadFile(path) //nolint:gosec // reason: path is a .go file from a repo walk, not user input
		if rerr != nil {
			return rerr
		}
		for _, m := range gibberish.FindAllString(string(src), -1) {
			if strings.HasPrefix(m, "k8s") {
				continue
			}
			if _, ok := seen[m]; !ok {
				seen[m] = path
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("idiom walk: %w", walkErr)
	}
	out := make([]Issue, 0, len(seen))
	for n, f := range seen {
		out = append(out, Issue{File: f, Name: n})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func skipDir(name string) bool {
	return name == "vendor" || name == "testdata" || name == ".git" || name == "node_modules"
}

func Format(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	names := make([]string, 0, len(issues))
	for _, is := range issues {
		names = append(names, is.Name)
	}
	return fmt.Sprintf("hash-gibberish identifiers (rename to self-documenting names): %s",
		strings.Join(names, ", "))
}
