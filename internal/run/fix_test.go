//nolint:testpackage // reason: exercises the production fix transaction and golangci argument builder
package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/config"
)

const (
	fixtureConfig = "fixture.yml"
	goWorkEnv     = "GOWORK"
	goWorkOff     = "off"
	brokenReport  = `{"Issues":[{"FromLinter":"brokenfix","Text":"broken edit","Pos":{"Filename":"shapes.go","Line":1}}]}`
)

func copyAutofixFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	err := os.CopyFS(root, os.DirFS("testdata/autofix"))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func writeAutofixConfig(t *testing.T) string {
	t.Helper()
	raw := strings.Replace(string(config.GolangCI), "  default: all", "  default: none\n  enable:\n    - noinlineerr", 1)
	if !strings.Contains(raw, "severity:\n  default: error") {
		t.Fatal("the fixture must inherit the production error severity")
	}
	path, err := filepath.Abs(fixtureConfig)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(path, []byte(raw), configMode)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func TestAutofixExcludesOnlyTheBrokenFix(t *testing.T) {
	t.Parallel()
	fix := golangciArgs(fixtureConfig, true, false)
	check := golangciArgs(fixtureConfig, false, false)
	at := slices.Index(fix, disableFlag)
	if at < 0 || at+1 >= len(fix) || fix[at+1] != noinlineerrLinter || !slices.Contains(fix, fixFlag) {
		t.Fatalf("the write pass must exclude noinlineerr: %v", fix)
	}
	if slices.Contains(check, fixFlag) || slices.Contains(check, disableFlag) ||
		slices.Contains(check, noinlineerrLinter) {
		t.Fatalf("the check pass must retain the complete configured rule set: %v", check)
	}
}

func TestAutofixFixtureCompilesAndCheckReportsNoinlineerr(t *testing.T) {
	root := copyAutofixFixture(t)
	t.Chdir(root)
	t.Setenv(goWorkEnv, goWorkOff)
	cfg := writeAutofixConfig(t)
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	err := g.safeFix()
	if err != nil {
		t.Fatalf("fix must leave all four shapes compiling without rollback: %v", err)
	}
	result := collectGolangci(t.Context(), golangciArgs(cfg, false, false))
	if len(result.notes) != 0 {
		t.Fatalf("check could not run: %v", result.notes)
	}
	reported := 0
	for _, finding := range result.diags {
		if finding.Linter == noinlineerrLinter {
			reported++
		}
	}
	if reported != 4 {
		t.Fatalf("check must report all four noinlineerr initializers after fix: %v", result.diags)
	}
	compileErr := verifyFix(t.Context(), root)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
}

func TestAutofixRollsBackEveryWriteAndNamesTheLinter(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	assertAutofixRollback(t,
		"printf 'package autofix\\nfunc Broken( {\\n' > shapes.go\n"+
			"printf 'package autofix\\nvar BrokenTest = missing\\n' > shapes_test.go\n"+
			"rm go.mod\nchmod 644 fixture.yml\nprintf 'new' > created.go\n")
}

func TestAutofixVetRejectsBrokenTestSource(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	assertAutofixRollback(t, "printf 'package autofix\\nvar BrokenTest = missing\\n' > shapes_test.go\n")
}

func assertAutofixRollback(t *testing.T, mutation string) {
	t.Helper()
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	toolDir := t.TempDir()
	t.Setenv("GOBIN", toolDir)
	script := "#!/bin/sh\ncase \" $* \" in\n*' --fix '*)\n" +
		mutation + ";;\nesac\n" +
		"printf '%s\\n' '" + brokenReport + "'\n"
	tool := filepath.Join(toolDir, golangciBin)
	err := os.WriteFile(tool, []byte(script), configMode)
	if err != nil {
		t.Fatal(err)
	}
	err = os.Chmod(tool, privateDirMode)
	if err != nil {
		t.Fatal(err)
	}
	before, snapshotErr := snapshotTree(root)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	err = g.safeFix()
	if err == nil {
		t.Fatal("a corrupting fixer must fail")
	}
	if !errors.Is(err, ErrGate) || !strings.Contains(err.Error(), "offending linters: brokenfix") {
		t.Fatalf("a corrupting fixer must fail and identify its linter: %v", err)
	}
	assertSnapshotRestored(t, before, root)
	compileErr := verifyFix(t.Context(), root)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
}

func assertSnapshotRestored(t *testing.T, before fixSnapshot, root string) {
	t.Helper()
	after, afterErr := snapshotTree(root)
	if afterErr != nil {
		t.Fatal(afterErr)
	}
	if len(before) != len(after) {
		t.Fatalf("rollback must remove created files: before %d, after %d", len(before), len(after))
	}
	for path, saved := range before {
		restored, exists := after[path]
		if !exists || !bytes.Equal(saved.data, restored.data) || saved.mode != restored.mode {
			t.Errorf("rollback must restore bytes and permissions of %s", path)
		}
	}
}
