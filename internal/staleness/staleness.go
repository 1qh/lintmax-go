package staleness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	httpTimeout     = 15 * time.Second
	toleranceWindow = 7 * 24 * time.Hour //nolint:mnd // tolerance window: skip releases <7d old to avoid racing registries
	hoursPerDay     = 24
	maxIssueRender  = 50
	envToken        = "GITHUB_TOKEN" //nolint:gosec // env var name, not a credential
	envSkip         = "LINTMAX_SKIP_STALENESS"
	envTolerance    = "LINTMAX_STALENESS_TOLERANCE_DAYS"
)

type Issue struct {
	ReleasedAt time.Time `json:"releasedAt"`
	Source     string    `json:"source"`
	Name       string    `json:"name"`
	Have       string    `json:"have"`
	Latest     string    `json:"latest"`
}

var rxActionPin = regexp.MustCompile(`uses:\s+([\w./-]+)@(v\d[\d.]*)`)

var errSkip = errors.New("staleness scan skipped")

func Scan(ctx context.Context, root string) ([]Issue, error) {
	if os.Getenv(envSkip) == "1" {
		return nil, nil
	}
	var (
		actionsOut []Issue
		gomodOut   []Issue
	)
	var wg sync.WaitGroup
	wg.Go(func() { actionsOut = scanActions(ctx, root) })
	wg.Go(func() { gomodOut = scanGoMod(ctx, root) })
	wg.Wait()
	merged := make([]Issue, 0, len(actionsOut)+len(gomodOut))
	merged = append(merged, actionsOut...)
	merged = append(merged, gomodOut...)
	return merged, nil
}

func tolerance() time.Duration {
	if v := os.Getenv(envTolerance); v != "" {
		var n int
		_, err := fmt.Sscanf(v, "%d", &n)
		if err == nil && n >= 0 {
			return time.Duration(n) * hoursPerDay * time.Hour
		}
	}
	return toleranceWindow
}

func scanActions(ctx context.Context, root string) []Issue {
	dir := filepath.Join(root, ".github", "workflows")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	return checkActionVersions(ctx, collectActionPins(dir, entries))
}

func collectActionPins(dir string, entries []os.DirEntry) map[string]string {
	seen := map[string]string{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yml") && !strings.HasSuffix(name, ".yaml") {
			continue
		}
		body, rerr := os.ReadFile(filepath.Join(dir, name)) //nolint:gosec // operator-supplied workflow paths
		if rerr != nil {
			continue
		}
		for _, m := range rxActionPin.FindAllStringSubmatch(string(body), -1) {
			if strings.HasPrefix(m[1], "./") {
				continue
			}
			seen[m[1]] = m[2]
		}
	}
	return seen
}

func checkActionVersions(ctx context.Context, pins map[string]string) []Issue {
	type res struct{ issue Issue }
	tol := tolerance()
	ch := make(chan res, len(pins))
	var wg sync.WaitGroup
	for action, have := range pins {
		wg.Go(func() {
			latest, releasedAt, err := fetchLatestRelease(ctx, action)
			if err != nil {
				return
			}
			if normalizeMajor(have) == normalizeMajor(latest) {
				return
			}
			if time.Since(releasedAt) < tol {
				return
			}
			ch <- res{issue: Issue{
				Source: "actions", Name: action, Have: have, Latest: latest, ReleasedAt: releasedAt,
			}}
		})
	}
	wg.Wait()
	close(ch)
	out := make([]Issue, 0, len(pins))
	for r := range ch {
		out = append(out, r.issue)
	}
	return out
}

func normalizeMajor(tag string) string {
	t := strings.TrimPrefix(tag, "v")
	if i := strings.IndexAny(t, ".+- "); i > 0 {
		t = t[:i]
	}
	return "v" + t
}

type ghRelease struct {
	TagName     string    `json:"tag_name"`     //nolint:tagliatelle // GitHub API wire shape
	PublishedAt time.Time `json:"published_at"` //nolint:tagliatelle // GitHub API wire shape
	Prerelease  bool      `json:"prerelease"`
}

func fetchLatestRelease(ctx context.Context, action string) (string, time.Time, error) {
	owner, name, ok := strings.Cut(action, "/")
	if !ok {
		return "", time.Time{}, fmt.Errorf("%w: action shape %q", errSkip, action)
	}
	if i := strings.Index(name, "/"); i > 0 {
		name = name[:i]
	}
	cctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()
	rel, err := doReleaseFetch(cctx, owner, name, action)
	if err != nil {
		return "", time.Time{}, err
	}
	if rel.Prerelease || rel.TagName == "" {
		return "", time.Time{}, fmt.Errorf("%w: %s no stable release", errSkip, action)
	}
	return rel.TagName, rel.PublishedAt, nil
}

func doReleaseFetch(ctx context.Context, owner, name, action string) (*ghRelease, error) {
	if cached, ok := readCachedRelease(action); ok {
		return cached, nil
	}
	return doReleaseFetchUncached(ctx, owner, name, action)
}

func doReleaseFetchUncached(ctx context.Context, owner, name, action string) (*ghRelease, error) {
	url := "https://api.github.com/repos/" + owner + "/" + name + "/releases/latest"
	req, rerr := http.NewRequestWithContext(ctx, http.MethodGet, url, http.NoBody)
	if rerr != nil {
		return nil, fmt.Errorf("request: %w", rerr)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv(envToken); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, derr := (&http.Client{Timeout: httpTimeout}).Do(req)
	if derr != nil {
		return nil, fmt.Errorf("fetch: %w", derr)
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: %s nil response", errSkip, action)
	}
	defer func() { _ = resp.Body.Close() }() //nolint:errcheck // close in defer
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s status %d", errSkip, action, resp.StatusCode)
	}
	var rel ghRelease
	jerr := json.NewDecoder(resp.Body).Decode(&rel)
	if jerr != nil {
		return nil, fmt.Errorf("decode: %w", jerr)
	}
	writeCachedRelease(action, &rel)
	return &rel, nil
}

func Format(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	bound := min(len(issues), maxIssueRender)
	for _, s := range issues[:bound] {
		age := int(time.Since(s.ReleasedAt).Hours() / hoursPerDay)
		fmt.Fprintf(&b, "  %-12s %-40s %s → %s (released %dd ago)\n",
			s.Source, s.Name, s.Have, s.Latest, age)
	}
	if len(issues) > bound {
		fmt.Fprintf(&b, "  ... %d more\n", len(issues)-bound)
	}
	return b.String()
}
