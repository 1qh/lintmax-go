package staleness

import (
	"context"
	"encoding/json"
	"os/exec"
	"strings"
	"time"
)

//nolint:tagliatelle // go list -json wire shape uses PascalCase
type goModEntry struct {
	Path     string       `json:"Path"`
	Version  string       `json:"Version"`
	Update   *goModUpdate `json:"Update"`
	Indirect bool         `json:"Indirect"`
	Main     bool         `json:"Main"`
}

//nolint:tagliatelle // go list -json wire shape uses PascalCase
type goModUpdate struct {
	Time    time.Time `json:"Time"`
	Version string    `json:"Version"`
}

func scanGoMod(ctx context.Context, root string) []Issue {
	sumKey := goSumKey(root)
	if cached, ok := readCachedGoMod(sumKey); ok {
		return cached
	}
	cctx, cancel := context.WithTimeout(ctx, 2*httpTimeout) //nolint:mnd // 2x http timeout for proxy-go resolve
	defer cancel()
	cmd := exec.CommandContext(cctx, "go", "list", "-m", "-u", "-json", "-mod=mod", "all")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	issues := parseGoModUpdates(out, usedModules(cctx, root))
	writeCachedGoMod(sumKey, issues)
	return issues
}

// usedModules returns the set of module paths the main module's packages actually
// compile. Modules absent here are graph-only noise (deps-of-deps that `go mod why`
// reports as "main module does not need") — enforcing latest on them is meaningless.
func usedModules(ctx context.Context, root string) map[string]bool {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", "./...")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	used := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		if p := strings.TrimSpace(line); p != "" {
			used[p] = true
		}
	}
	return used
}

func parseGoModUpdates(out []byte, used map[string]bool) []Issue {
	tol := tolerance()
	dec := json.NewDecoder(strings.NewReader(string(out)))
	result := make([]Issue, 0, 16) //nolint:mnd // initial capacity hint
	for dec.More() {
		var e goModEntry
		if dec.Decode(&e) != nil {
			break
		}
		if e.Indirect && len(used) > 0 && !used[e.Path] {
			continue
		}
		if e.Main || e.Update == nil || e.Update.Version == "" {
			continue
		}
		if tol > 0 && !e.Update.Time.IsZero() && time.Since(e.Update.Time) < tol {
			continue
		}
		src := "go.mod"
		if e.Indirect {
			src = "go.mod (indirect)"
		}
		result = append(result, Issue{
			Source: src, Name: e.Path,
			Have: e.Version, Latest: e.Update.Version, ReleasedAt: e.Update.Time,
		})
	}
	return result
}
