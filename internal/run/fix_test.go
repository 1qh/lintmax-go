//nolint:testpackage // reason: exercises the production fix transaction and golangci argument builder
package run

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/config"
	"github.com/1qh/lintmax-go/internal/diag"
)

const (
	fixtureConfig     = "fixture.yml"
	goWorkEnv         = "GOWORK"
	goWorkOff         = "off"
	brokenSource      = "package autofix\nvar Broken = missing\n"
	cleanFixer        = "acleanfix"
	brokenFixer       = "brokenfix"
	noopFixer         = "noopfix"
	replayPackage     = "nested"
	shapeSource       = "shapes.go"
	shapeTest         = "shapes_test.go"
	permissionsSource = "permissions/source.go"
	untouchedSource   = "untouched/source.go"
	testOnlySource    = "tests/source_test.go"
	goBinEnv          = "GOBIN"
)

const brokenReport = `{"Issues":[{"FromLinter":"brokenfix","Text":"broken edit",` +
	`"Pos":{"Filename":"shapes.go","Line":1}}]}`

var errReplayValidation = errors.New("./shapes.go:1: undefined: missing")

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
	t.Setenv(goBinEnv, toolDir)
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

func TestAutofixNamesOnlyTheBrokenCandidate(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	toolDir := t.TempDir()
	t.Setenv(goBinEnv, toolDir)
	report := `{"Issues":[` +
		`{"FromLinter":"brokenfix","Text":"broken edit","Pos":{"Filename":"shapes.go","Line":1}},` +
		`{"FromLinter":"cleanfix","Text":"clean edit","Pos":{"Filename":"shapes_test.go","Line":1}}]}`
	script := "#!/bin/sh\ncase \" $* \" in\n" +
		"*' --fix '*' --enable-only cleanfix '*) printf 'package autofix\\nvar Clean = 1\\n' > clean.go;;\n" +
		"*' --fix '*) printf 'package autofix\\nvar Broken = missing\\n' > broken.go;;\n" +
		"esac\nprintf '%s\\n' '" + report + "'\n"
	tool := filepath.Join(toolDir, golangciBin)
	err := os.WriteFile(tool, []byte(script), privateDirMode)
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
		t.Fatal("the broken candidate must fail the fix transaction")
	}
	if !errors.Is(err, ErrGate) || !strings.Contains(err.Error(), "(offending linters: brokenfix)") {
		t.Fatalf("only the broken candidate must be named: %v", err)
	}
	assertSnapshotRestored(t, before, root)
	compileErr := verifyFix(t.Context(), root)
	if compileErr != nil {
		t.Fatal(compileErr)
	}
}

type replayTrace struct {
	commands []string
	linters  []string
	root     string
	snapshot fixSnapshot
	test     *testing.T
}

func (trace *replayTrace) command(ctx context.Context, root, program string, args ...string) ([]byte, error) {
	t := trace.test
	t.Helper()
	if program == goCmd {
		trace.commands = append(trace.commands, strings.Join(args, " "))
		return runFixCommand(ctx, root, program, args...)
	}
	if trace.root != "" && trace.root != root {
		t.Fatal("diagnosis must reuse one replay directory")
	}
	trace.root = root
	return trace.applyLinter(root, args...)
}

func (trace *replayTrace) applyLinter(root string, args ...string) ([]byte, error) {
	t := trace.test
	t.Helper()
	assertReplayRestored(t, trace.snapshot, root)
	at := slices.Index(args, "--enable-only")
	if at < 0 || at+1 >= len(args) {
		t.Fatalf("missing candidate selection: %v", args)
	}
	name := args[at+1]
	trace.linters = append(trace.linters, name)
	var path, source string
	switch name {
	case brokenFixer:
		path, source = "broken.go", brokenSource
	case cleanFixer:
		path, source = "nested/clean.go", "package nested\nvar Clean = 1\n"
	case noopFixer:
		return nil, nil
	default:
	}
	if path == "" {
		t.Fatalf("unexpected candidate: %s", name)
	}
	err := os.WriteFile(filepath.Join(root, path), []byte(source), configMode)
	if err != nil {
		t.Fatal(err)
	}
	return nil, nil
}

func assertReplayRestored(t *testing.T, before fixSnapshot, root string) {
	t.Helper()
	after, err := snapshotTree(root)
	if err != nil {
		t.Fatal(err)
	}
	delete(after, replayConfigFile)
	if len(after) != len(before) {
		t.Fatalf("each replay must begin from the snapshot: %d files, want %d", len(after), len(before))
	}
	for path, saved := range before {
		now, exists := after[path]
		if !exists || !bytes.Equal(now.data, saved.data) || now.mode != saved.mode {
			t.Fatalf("previous candidate's changes leaked into %s", path)
		}
	}
}

func TestAutofixReplayBuildCountsAndPackageScope(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	err := os.Mkdir(filepath.Join(root, replayPackage), privateDirMode)
	if err != nil {
		t.Fatal(err)
	}
	err = os.WriteFile(filepath.Join(root, replayPackage, "source.go"), []byte("package nested\n"), configMode)
	if err != nil {
		t.Fatal(err)
	}
	trace := &replayTrace{commands: nil, linters: nil, root: "", snapshot: nil, test: t}
	command := trace.command
	baselineErr := verifyFixPackages(t.Context(), root, []string{allPackages}, command)
	if baselineErr != nil {
		t.Fatal(baselineErr)
	}
	snapshot, snapshotErr := snapshotTree(root)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	trace.snapshot = snapshot
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	findings := []diag.Diagnostic{
		{File: "nested/source.go", Linter: cleanFixer, Rule: "", Line: 1},
		{File: shapeSource, Linter: brokenFixer, Rule: "", Line: 1},
		{File: shapeTest, Linter: noopFixer, Rule: "", Line: 1},
	}
	validationErr := errReplayValidation
	culprits := g.offendingFixers(verifiedFixSnapshot{files: snapshot}, findings, validationErr, command)
	if culprits != offendingLinters+brokenFixer {
		t.Fatalf("exact culprit set: %s", culprits)
	}
	want := []string{"build ./...", "vet ./...", "build ./.", "build ./nested", "vet ./nested"}
	if !slices.Equal(trace.commands, want) {
		t.Fatalf("one baseline build, two scoped builds and zero noop builds: got %v, want %v", trace.commands, want)
	}
	if !slices.Equal(trace.linters, []string{brokenFixer, cleanFixer, noopFixer}) {
		t.Fatalf("file priority must retain all candidates: %v", trace.linters)
	}
	_, statErr := os.Stat(trace.root)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("diagnosis must remove its replay directory: %v", statErr)
	}
	assertSnapshotRestored(t, snapshot, root)
}

func TestAutofixChangedPackagesIncludeTestsAndDeletions(t *testing.T) {
	t.Parallel()
	saved := savedFile{data: []byte("original"), mode: configMode}
	changed := savedFile{data: []byte("changed"), mode: configMode}
	before := fixSnapshot{
		fixtureConfig:       saved,
		permissionsSource:   saved,
		"removed/source.go": saved,
		shapeSource:         saved,
		shapeTest:           saved,
		testOnlySource:      saved,
		untouchedSource:     saved,
	}
	after := fixSnapshot{
		"created/source.go": changed,
		fixtureConfig:       changed,
		permissionsSource:   {data: saved.data, mode: privateDirMode},
		shapeSource:         changed,
		shapeTest:           changed,
		testOnlySource:      changed,
		untouchedSource:     saved,
	}
	want := []string{"./.", "./created", "./removed", "./tests"}
	packages := changedFixPackages(before, after)
	if !slices.Equal(packages, want) {
		t.Fatalf("deduplicate changed Go package dirs, including removed and test files: got %v, want %v", packages, want)
	}
	if unchanged := changedFixPackages(before, before); len(unchanged) != 0 {
		t.Fatalf("unchanged files must not trigger validation: %v", unchanged)
	}
}
