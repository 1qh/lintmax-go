package repo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAMinifiedBundleIsExcludedFromTheSpellCheck(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"*.min.js", "*.min.css"} {
		found := false
		for _, got := range minified() {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("a spell-checker over minified identifiers reports only noise, so %q must be excluded", want)
		}
	}
}

func TestTheFormatterExcludesEveryMinifiedShapeTheSpellCheckDoes(t *testing.T) {
	t.Parallel()
	all := strings.Join(excludes(), " ")
	for _, one := range minified() {
		if !strings.Contains(all, "**/"+one) {
			t.Fatalf("the formatter and the spell check must exclude the same generated files, or one rewrites what the other refuses: %q", one)
		}
	}
}

func TestUnauthoredTreesAreExcludedFromTheSpellCheck(t *testing.T) {
	t.Parallel()
	for _, want := range []string{"testdata", "vendor", "node_modules"} {
		found := false
		for _, got := range unauthored() {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("a fixture tree is INPUT rather than authored prose, so %q must be excluded or every foreign word in a fixture reads as a typo", want)
		}
	}
}

func TestAProjectsOwnTyposConfigWins(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	cfg := t.TempDir()
	if got := typosConfig(root, cfg); got != filepath.Join(cfg, typosFile) {
		t.Fatalf("a project carrying no config must get the generated one, or the check silently stops running: %q", got)
	}
	own := filepath.Join(root, typosFile)
	if err := os.WriteFile(own, []byte("[default]\n"), configMode); err != nil {
		t.Fatalf("write the project config: %v", err)
	}
	if got := typosConfig(root, cfg); got != own {
		t.Fatalf("a project must be able to supply its own domain vocabulary, or a real product term reads as a typo for ever: %q", got)
	}
}
