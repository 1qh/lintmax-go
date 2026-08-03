package repo_test

import (
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/repo"
)

const (
	hostURL  = "https://plugins.dprint.dev/"
	official = "dprint/"
	gplane   = "g-plane/"
	lax      = "bartlomieju/"
)

func TestPluginPathKeepsAHyphenatedName(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"json-0.23.0.wasm":                 official + "json",
		"toml-0.7.0.wasm":                  official + "toml",
		gplane + "pretty_yaml-v0.6.0.wasm": gplane + "pretty_yaml",
		gplane + "markup_fmt-v0.27.3.wasm": gplane + "markup_fmt",
		lax + "lax-sql-0.3.0.wasm":         lax + "lax-sql",
	}
	for pinned, want := range cases {
		got := repo.PluginPath(hostURL + pinned)
		if got != want {
			t.Errorf("%s resolved to %q, want %q", pinned, got, want)
		}
	}
}

func TestPluginPathRefusesAForeignHost(t *testing.T) {
	t.Parallel()
	if got := repo.PluginPath("https://example.invalid/json-0.23.0.wasm"); got != "" {
		t.Fatalf("want no path for a foreign host, got %q", got)
	}
}

func TestSeedCoversEveryFileTypeTheCollectionFormats(t *testing.T) {
	t.Parallel()
	want := []string{
		official + "json", official + "markdown", official + "toml", official + "dockerfile",
		gplane + "pretty_yaml", gplane + "malva", gplane + "markup_fmt",
		gplane + "pretty_graphql", lax + "lax-sql",
	}
	seen := make(map[string]bool)
	for _, pinned := range repo.Seed() {
		seen[repo.PluginPath(pinned)] = true
	}
	for _, path := range want {
		if !seen[path] {
			t.Errorf("the seed no longer carries %s", path)
		}
	}
}

func TestConfigGeneratesBothFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	got, err := repo.Config(t.Context(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, dir) {
		t.Fatalf("want the config under %s, got %s", dir, got)
	}
}
