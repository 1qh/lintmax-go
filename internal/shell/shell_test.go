package shell_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/shell"
)

const (
	clean    = "#!/bin/sh\nmain() {\n  echo ok\n}\nmain\n"
	broken   = "#!/bin/sh\nif [ $1 = x ]; then\necho \"$UNSET_ONE\"\nfi\n"
	ownName  = "own.sh"
	toolName = "shellcheck"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
	if err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func TestScriptsSkipsVendoredTrees(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, ownName, clean)
	nested := filepath.Join(dir, "node_modules")
	err := os.Mkdir(nested, 0o750)
	if err != nil {
		t.Fatal(err)
	}
	write(t, nested, "theirs.sh", clean)
	found, scanErr := shell.Scripts(dir)
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	if len(found) != 1 || !strings.HasSuffix(found[0], ownName) {
		t.Fatalf("want only the repository's own script, got %v", found)
	}
}

func TestGateIsSilentWhenNoScriptExists(t *testing.T) {
	t.Setenv("LINTMAX_ALL_FILES", "1")
	notes := shell.Gate(t.Context(), t.TempDir(), false)
	if notes != nil {
		t.Fatalf("want no notes for a tree with no scripts, got %v", notes)
	}
}

func TestGateReportsAPlantedDefect(t *testing.T) {
	t.Setenv("LINTMAX_ALL_FILES", "1")
	dir := t.TempDir()
	write(t, dir, "bad.sh", broken)
	notes := shell.Gate(t.Context(), dir, false)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, toolName) {
		t.Fatalf("an unquoted expansion and an undefined variable must reach %s, got %v", toolName, notes)
	}
}

func TestGateSaysSoWhenATrueToolIsAbsent(t *testing.T) {
	t.Setenv("LINTMAX_ALL_FILES", "1")
	dir := t.TempDir()
	write(t, dir, "one.sh", clean)
	t.Setenv("PATH", t.TempDir())
	notes := shell.Gate(t.Context(), dir, false)
	joined := strings.Join(notes, "\n")
	if !strings.Contains(joined, "not installed") || !strings.Contains(joined, "1 shell script") {
		t.Fatalf("an absent tool must report what went unchecked, got %v", notes)
	}
}

// The stage judges nothing until a repository opts in, so a consumer that has not ground its shell
// baseline is never blocked by a finding its change did not introduce.
func TestGateIsSilentUntilTheRepositoryOptsIn(t *testing.T) {
	t.Setenv("LINTMAX_ALL_FILES", "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "planted.sh"), []byte("#!/bin/sh\necho $undefined\n"), 0o600); err != nil {
		t.Fatalf("planting a script: %v", err)
	}
	if notes := shell.Gate(t.Context(), dir, false); len(notes) != 0 {
		t.Errorf("an opted-out repository must be judged by nothing, got %v", notes)
	}
}
