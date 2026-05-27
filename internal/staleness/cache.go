package staleness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	cacheTTL      = 24 * time.Hour
	cacheDirPerm  = 0o750
	cacheFilePerm = 0o600
)

type cachedRelease struct {
	Release  ghRelease `json:"release"`
	CachedAt time.Time `json:"cachedAt"`
}

func cachePath(action string) string {
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	safe := strings.ReplaceAll(action, "/", "_")
	return filepath.Join(dir, "lintmax-go", "staleness", safe+".json")
}

func readCachedRelease(action string) (*ghRelease, bool) {
	p := cachePath(action)
	if p == "" {
		return nil, false
	}
	raw, err := os.ReadFile(p) //nolint:gosec // path derived from os.UserCacheDir + sanitized action
	if err != nil {
		return nil, false
	}
	var c cachedRelease
	if json.Unmarshal(raw, &c) != nil {
		return nil, false
	}
	if time.Since(c.CachedAt) > cacheTTL {
		return nil, false
	}
	return &c.Release, true
}

func goSumKey(root string) string {
	raw, err := os.ReadFile(filepath.Join(root, "go.sum")) //nolint:gosec // local repo file
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func goModCachePath(key string) string {
	if key == "" {
		return ""
	}
	dir, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "lintmax-go", "staleness", "gomod-"+key+".json")
}

type cachedGoMod struct {
	Issues   []Issue   `json:"issues"`
	CachedAt time.Time `json:"cachedAt"`
}

func readCachedGoMod(key string) ([]Issue, bool) {
	p := goModCachePath(key)
	if p == "" {
		return nil, false
	}
	raw, err := os.ReadFile(p) //nolint:gosec // path derived from os.UserCacheDir + sha256 key
	if err != nil {
		return nil, false
	}
	var c cachedGoMod
	if json.Unmarshal(raw, &c) != nil {
		return nil, false
	}
	if time.Since(c.CachedAt) > cacheTTL {
		return nil, false
	}
	return c.Issues, true
}

func writeCachedGoMod(key string, issues []Issue) {
	p := goModCachePath(key)
	if p == "" {
		return
	}
	mkErr := os.MkdirAll(filepath.Dir(p), cacheDirPerm)
	if mkErr != nil {
		return
	}
	now := time.Now() //nolint:forbidigo // reason: bootstrap clock read
	raw, err := json.Marshal(cachedGoMod{Issues: issues, CachedAt: now})
	if err != nil {
		return
	}
	_ = os.WriteFile(p, raw, cacheFilePerm) //nolint:errcheck // best-effort cache
}

func writeCachedRelease(action string, rel *ghRelease) {
	p := cachePath(action)
	if p == "" {
		return
	}
	mkErr := os.MkdirAll(filepath.Dir(p), cacheDirPerm)
	if mkErr != nil {
		return
	}
	now := time.Now() //nolint:forbidigo // reason: bootstrap clock read
	raw, err := json.Marshal(cachedRelease{Release: *rel, CachedAt: now})
	if err != nil {
		return
	}
	_ = os.WriteFile(p, raw, cacheFilePerm) //nolint:errcheck // best-effort cache
}
