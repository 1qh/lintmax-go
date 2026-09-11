package repo //nolint:testpackage // reason: exercises unexported minified

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const artifactCSSName = "app.css"

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
			t.Fatalf(
				"the formatter and spell check must exclude the same generated file: %q",
				one,
			)
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
			t.Fatalf(
				"fixture tree %q must be excluded from spell checking unauthored prose",
				want,
			)
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
	err := os.WriteFile(own, []byte("[default]\n"), configMode)
	if err != nil {
		t.Fatalf("write the project config: %v", err)
	}
	if got := typosConfig(root, cfg); got != own {
		t.Fatalf(
			"a project's own domain vocabulary must take precedence: %q",
			got,
		)
	}
}

func TestAGeneratedArtifactIsExcludedFromTheFormatter(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	banner := filepath.Join(root, artifactCSSName)
	err := os.WriteFile(banner, []byte("/*! tailwindcss v4 */\n.a{color:red}"), configMode)
	if err != nil {
		t.Fatalf("write the generated stylesheet: %v", err)
	}
	authored := filepath.Join(root, "hand.css")
	err = os.WriteFile(authored, []byte(".a { color: red }\n"), configMode)
	if err != nil {
		t.Fatalf("write the authored stylesheet: %v", err)
	}
	found := generated(root)
	if len(found) != 1 || found[0] != artifactCSSName {
		t.Fatalf(
			"a file whose banner says it is generated must be excluded from formatting: %v",
			found,
		)
	}
}

func TestEveryExcludedArtifactReachesTheFormatterInvocation(t *testing.T) {
	t.Parallel()
	args := dprintArgs("check", "/tmp/cfg.json", []string{"server/assets/app.css", "server/assets/collection.js"})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--excludes server/assets/app.css server/assets/collection.js") {
		t.Fatalf(
			"every exclusion must reach the formatter under ONE flag, because dprint refuses a repeated one outright: %v",
			args,
		)
	}
	if strings.Count(joined, "--excludes") != 1 {
		t.Fatalf(
			"dprint refuses `--excludes` used more than once, so a repeated flag fails the stage instead of excluding: %v",
			args,
		)
	}
}

func TestTheWholeTreeStagesAreOptInUntilAskedFor(t *testing.T) {
	t.Setenv(allFilesEnv, "")
	if WholeTreeRequested() {
		t.Fatal(
			"whole-tree stages must stay disabled until the repository requests them",
		)
	}
	if notes := Gate(t.Context(), t.TempDir(), false); notes != nil {
		t.Fatalf("an unasked whole-tree stage must contribute no finding at all: %v", notes)
	}
	t.Setenv(allFilesEnv, "1")
	if !WholeTreeRequested() {
		t.Fatal("a repository that asks for the whole-tree stages must get them, or the capability is unreachable")
	}
}
