package idiom

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
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
		return scanEntry(seen, path, d, err)
	})
	if walkErr != nil {
		return nil, fmt.Errorf("idiom walk: %w", walkErr)
	}
	out := make([]Issue, 0, len(seen))
	for name, file := range seen {
		out = append(out, Issue{File: file, Name: name})
	}
	slices.SortFunc(out, func(a, b Issue) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func scanEntry(seen map[string]string, path string, d os.DirEntry, err error) error {
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
	src, rerr := os.ReadFile(path) //nolint:gosec // reason: path is a .go file from the repo walk, not user input
	if rerr != nil {
		return fmt.Errorf("read %s: %w", path, rerr)
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
	return "hash-gibberish identifiers (rename to self-documenting names): " + strings.Join(names, ", ")
}

func ScanScripts(ctx context.Context, root string) ([]string, error) {
	//nolint:gosec // reason: fixed git argv; root is the repo path being linted, not user input
	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-s").Output()
	if err != nil {
		return nil, nil //nolint:nilerr // reason: git absent / not a repo → no scripts to check, not a gate error
	}
	var bad []string
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 || !strings.HasSuffix(fields[3], ".sh") {
			continue
		}
		if !strings.HasPrefix(line, "100755") {
			bad = append(bad, fields[3])
		}
	}
	slices.Sort(bad)
	return bad, nil
}

func FormatScripts(bad []string) string {
	if len(bad) == 0 {
		return ""
	}
	return "shell scripts not executable (git mode must be 100755 — run `chmod +x` and `git update-index --chmod=+x`): " +
		strings.Join(bad, ", ")
}
