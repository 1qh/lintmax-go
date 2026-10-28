//nolint:testpackage // reason: exercises the production fix transaction and golangci argument builder
package run

import (
	"bytes"
	"context"
	"errors"
	"io"
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
	largeFixture      = "large.bin"
	fixerNotes        = "notes.txt"
	createdNotes      = "created.txt"
	createdSource     = "created.go"
	embedData         = "nested/data.txt"
	nestedSource      = "nested/source.go"
	embedContents     = "embedded data"
)

const (
	brokenCulpritLabel = "(offending linters: brokenfix)"
	rootBuildCommand   = "build ./."
	nestedBuildCommand = "build ./nested"
	nestedVetCommand   = "vet ./nested"
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
	if at < 0 || at+1 >= len(fix) || fix[at+1] != unsafeAutofixLinters ||
		!slices.Contains(fix, fixFlag) {
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
	if !errors.Is(err, ErrGate) || !strings.Contains(err.Error(), brokenCulpritLabel) {
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
		{File: nestedSource, Linter: cleanFixer, Rule: "", Line: 1},
		{File: shapeSource, Linter: brokenFixer, Rule: "", Line: 1},
		{File: shapeTest, Linter: noopFixer, Rule: "", Line: 1},
	}
	validationErr := errReplayValidation
	culprits := g.offendingFixers(verifiedFixSnapshot{files: snapshot}, findings, validationErr, command)
	if culprits != offendingLinters+brokenFixer {
		t.Fatalf("exact culprit set: %s", culprits)
	}
	want := []string{"build ./...", "vet ./...", rootBuildCommand, nestedBuildCommand, nestedVetCommand}
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

func writeFixFixture(t *testing.T, root, path string, data []byte) {
	t.Helper()
	err := writeSaved(root, path, savedFile{data: data, mode: configMode})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAutofixSnapshotIncludesOnlyWritableFiles(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	want := []string{
		fileGoMod, fileGoSum, fileGoWork, fileGoWorkSum,
		"nested/go.mod", "nested/go.sum", "nested/go.work", "nested/go.work.sum", "nested/source_test.go", shapeSource,
	}
	for _, path := range want {
		writeFixFixture(t, root, path, []byte("saved"))
	}
	for _, path := range []string{".git/ignored.go", "nested/node_modules/ignored.go", "source.go.bak", fixerNotes} {
		writeFixFixture(t, root, path, []byte("ignored"))
	}
	snapshot, err := snapshotTree(root)
	if err != nil {
		t.Fatal(err)
	}
	paths := make([]string, 0, len(snapshot))
	for path := range snapshot {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	slices.Sort(want)
	if !slices.Equal(paths, want) {
		t.Fatalf("snapshot must contain only writable files outside skipped directories: got %v, want %v", paths, want)
	}
}

func TestAutofixRollbackPreservesNonGoFixerWrites(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	large := bytes.Repeat([]byte("fixture"), 1<<20)
	writeFixFixture(t, root, largeFixture, large)
	writeFixFixture(t, root, fixerNotes, []byte("before"))
	before, snapshotErr := snapshotTree(root)
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	for _, path := range []string{largeFixture, fixerNotes, fixtureConfig} {
		if _, exists := before[path]; exists {
			t.Fatalf("non-Go file must not enter the snapshot: %s", path)
		}
	}
	toolDir := t.TempDir()
	t.Setenv(goBinEnv, toolDir)
	script := "#!/bin/sh\ncase \" $* \" in\n*' --fix '*)\n" +
		"printf 'after' > notes.txt\nprintf 'new' > created.txt\n" +
		"printf 'package autofix\\nvar Broken = missing\\n' > created.go;;\n" +
		"esac\nprintf '%s\\n' '" + brokenReport + "'\n"
	writeFixFixture(t, toolDir, golangciBin, []byte(script))
	chmodErr := os.Chmod(filepath.Join(toolDir, golangciBin), privateDirMode)
	if chmodErr != nil {
		t.Fatal(chmodErr)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	err := g.safeFix()
	if err == nil {
		t.Fatal("the corrupting fixer must fail")
	}
	if !errors.Is(err, ErrGate) || !strings.Contains(err.Error(), brokenCulpritLabel) {
		t.Fatalf("the failed pass must retain refusal and culprit attribution: %v", err)
	}
	assertSnapshotRestored(t, before, root)
	assertFixFixtureData(t, root, largeFixture, large)
	assertFixFixtureData(t, root, fixerNotes, []byte("after"))
	assertFixFixtureData(t, root, createdNotes, []byte("new"))
	_, statErr := os.Stat(filepath.Join(root, createdSource))
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("restore must remove created Go source: %v", statErr)
	}
}

func assertFixFixtureData(t *testing.T, root, path string, want []byte) {
	t.Helper()
	module, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeErr := module.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	})
	data, err := module.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, want) {
		t.Fatalf("unexpected contents of %s after replay or rollback", path)
	}
}

func TestAutofixReplayCopiesEmbedTargets(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	source := "package nested\nimport \"embed\"\n" + embedDirective + " data.txt\nvar Data string\n" +
		embedDirective + " \"spaced name.txt\" `raw name.txt` assets/*.txt tree all:hidden\nvar Files embed.FS\n"
	writeFixFixture(t, root, nestedSource, []byte(source))
	writeFixFixture(t, root, embedData, []byte(embedContents))
	for _, path := range []string{
		"nested/spaced name.txt", "nested/raw name.txt", "nested/assets/file.txt", "nested/tree/deep/file.txt",
		"nested/hidden/.secret",
	} {
		writeFixFixture(t, root, path, []byte("extra embed target"))
	}
	writeFixFixture(t, root, largeFixture, []byte("unneeded build output"))
	verifyErr := verifyFix(t.Context(), root)
	if verifyErr != nil {
		t.Fatal(verifyErr)
	}
	snapshot, err := snapshotTree(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := snapshot[embedData]; exists {
		t.Fatal("embed targets must be copied separately from the rollback snapshot")
	}
	trace := &replayTrace{commands: nil, linters: nil, root: "", snapshot: snapshot, test: t}
	command := func(ctx context.Context, replayRoot, program string, args ...string) ([]byte, error) {
		if program != goCmd {
			assertFixFixtureData(t, replayRoot, embedData, []byte(embedContents))
			_, statErr := os.Stat(filepath.Join(replayRoot, largeFixture))
			if !errors.Is(statErr, os.ErrNotExist) {
				t.Fatalf("replay must omit unrelated non-Go files: %v", statErr)
			}
		}
		return trace.command(ctx, replayRoot, program, args...)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	findings := []diag.Diagnostic{
		{File: shapeSource, Linter: brokenFixer, Rule: "", Line: 1},
		{File: nestedSource, Linter: cleanFixer, Rule: "", Line: 1},
	}
	culprits := g.offendingFixers(verifiedFixSnapshot{files: snapshot}, findings, errReplayValidation, command)
	if culprits != offendingLinters+brokenFixer {
		t.Fatalf("the compiling embed package must not be blamed: %s", culprits)
	}
	if !slices.Equal(trace.commands, []string{rootBuildCommand, nestedBuildCommand, nestedVetCommand}) {
		t.Fatalf("replay must compile and vet the package containing the embed: %v", trace.commands)
	}
}

func captureFixReport(t *testing.T, action func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stderr
	os.Stderr = writer
	t.Cleanup(func() { os.Stderr = original })
	actionErr := action()
	os.Stderr = original
	if closeErr := writer.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	out, readErr := io.ReadAll(reader)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if closeErr := reader.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
	return string(out), actionErr
}

func TestAutofixPerfsprintAddsImportAndReportsFindings(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	raw, err := os.ReadFile(cfg)
	if err != nil {
		t.Fatal(err)
	}
	raw = bytes.Replace(raw, []byte("    - noinlineerr"), []byte("    - noinlineerr\n    - perfsprint"), 1)
	writeFixFixture(t, root, fixtureConfig, raw)
	const source = "package autofix\nimport \"fmt\"\nfunc Number(intVar int) string { return fmt.Sprint(intVar) }\n"
	writeFixFixture(t, root, createdSource, []byte(source))
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	if fixErr := g.safeFix(); fixErr != nil {
		t.Fatal(fixErr)
	}
	data, readErr := os.ReadFile(createdSource)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if !bytes.Contains(data, []byte("strconv.Itoa(intVar)")) || !bytes.Contains(data, []byte("\"strconv\"")) {
		t.Fatalf("perfsprint must keep its rewrite and add the import: %s", data)
	}
	if compileErr := verifyFix(t.Context(), root); compileErr != nil {
		t.Fatal(compileErr)
	}
	remaining := collectGolangci(t.Context(), golangciArgs(cfg, false, false))
	out, reportErr := captureFixReport(t, func() error { return report(remaining.diags, remaining.notes) })
	if !errors.Is(reportErr, ErrGate) || !strings.Contains(out, "noinlineerr:") || strings.Contains(out, "undefined: strconv") {
		t.Fatalf("normal findings must survive the import repair: %s (%v)", out, reportErr)
	}
}

func TestAutofixRecoveryKeepsCompilingFixAndReportsOriginal(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	toolDir := t.TempDir()
	t.Setenv(goBinEnv, toolDir)
	const reportJSON = `{"Issues":[` +
		`{"FromLinter":"brokenfix","Text":"broken edit","Pos":{"Filename":"created.go","Line":2}},` +
		`{"FromLinter":"cleanfix","Text":"clean edit","Pos":{"Filename":"created.go","Line":3}}]}`
	script := "#!/bin/sh\ncase \" $* \" in\n" +
		"*' --fix '*' --enable-only cleanfix '*) printf 'package autofix\\nvar Clean = 2\\n' > created.go;;\n" +
		"*' --fix '*'brokenfix,cleanfix'*) ;;\n" +
		"*' --fix '*'" + unsafeAutofixLinters + ",brokenfix'*) printf 'package autofix\\nvar Clean = 2\\n' > created.go;;\n" +
		"*' --fix '*) printf 'package autofix\\nvar Broken = missing\\nvar Clean = 2\\n' > created.go;;\n" +
		"esac\nif [ -f created.go ] && ! grep -q missing created.go; then\n" +
		"printf '%s\\n' '{\"Issues\":[{\"FromLinter\":\"brokenfix\",\"Text\":\"broken edit\",\"Pos\":{\"Filename\":\"created.go\",\"Line\":2}}]}'\n" +
		"else\nprintf '%s\\n' '" + reportJSON + "'\nfi\n"
	writeFixFixture(t, toolDir, golangciBin, []byte(script))
	if err := os.Chmod(filepath.Join(toolDir, golangciBin), privateDirMode); err != nil {
		t.Fatal(err)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	out, fixErr := captureFixReport(t, g.safeFix)
	if !errors.Is(fixErr, ErrGate) || !strings.Contains(out, "brokenfix:2") || strings.Contains(out, "cleanfix:") ||
		!strings.Contains(out, "autofix reverted (offending linters: brokenfix)") {
		t.Fatalf("recovery must report the original finding and culprit: %s (%v)", out, fixErr)
	}
	assertFixFixtureData(t, root, createdSource, []byte("package autofix\nvar Clean = 2\n"))
	if compileErr := verifyFix(t.Context(), root); compileErr != nil {
		t.Fatal(compileErr)
	}
}

func TestAutofixUnattributedFailureRestoresAndReportsFindings(t *testing.T) {
	t.Setenv(goWorkEnv, goWorkOff)
	root := copyAutofixFixture(t)
	t.Chdir(root)
	cfg := writeAutofixConfig(t)
	toolDir := t.TempDir()
	t.Setenv(goBinEnv, toolDir)
	script := "#!/bin/sh\ncase \" $* \" in\n" +
		"*' --enable-only '*) ;;\n" +
		"*' --fix '*) printf 'package autofix\\nvar Broken = missing\\n' > created.go;;\n" +
		"esac\nprintf '%s\\n' '" + brokenReport + "'\n"
	writeFixFixture(t, toolDir, golangciBin, []byte(script))
	if err := os.Chmod(filepath.Join(toolDir, golangciBin), privateDirMode); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotTree(root)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: cfg, skipCheck: false, greenKey: ""}
	out, fixErr := captureFixReport(t, g.safeFix)
	if !errors.Is(fixErr, ErrGate) || !strings.Contains(out, "brokenfix:1") ||
		!strings.Contains(out, "autofix reverted (combined fixes; candidate linters: brokenfix)") {
		t.Fatalf("unattributed edits must retain original findings and explain rollback: %s (%v)", out, fixErr)
	}
	assertSnapshotRestored(t, before, root)
	if compileErr := verifyFix(t.Context(), root); compileErr != nil {
		t.Fatal(compileErr)
	}
}
