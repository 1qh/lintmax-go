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
	issues := parseGoModUpdates(out)
	writeCachedGoMod(sumKey, issues)
	return issues
}

func parseGoModUpdates(out []byte) []Issue {
	tol := tolerance()
	dec := json.NewDecoder(strings.NewReader(string(out)))
	result := make([]Issue, 0, 16) //nolint:mnd // initial capacity hint
	for dec.More() {
		var e goModEntry
		if dec.Decode(&e) != nil {
			break
		}
		if e.Main || e.Update == nil || e.Update.Version == "" {
			continue
		}
		if !e.Update.Time.IsZero() && time.Since(e.Update.Time) < tol {
			continue
		}
		result = append(result, Issue{
			Source: "go.mod", Name: e.Path,
			Have: e.Version, Latest: e.Update.Version, ReleasedAt: e.Update.Time,
		})
	}
	return result
}
