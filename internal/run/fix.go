package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/1qh/lintmax-go/internal/diag"
)

const (
	privateDirMode   = 0o700
	formattersKey    = "\nformatters:\n"
	replayConfigFile = ".lintmax-replay.yml"
	offendingLinters = "offending linters: "
	embedDirective   = "//go:embed"
	fileGoSum        = "go.sum"
	fileGoWork       = "go.work"
	fileGoWorkSum    = "go.work.sum"
)

var ErrSnapshotSource = errors.New("cannot snapshot non-regular Go source")

type savedFile struct {
	data []byte
	mode fs.FileMode
}

type fixSnapshot map[string]savedFile

type verifiedFixSnapshot struct {
	files fixSnapshot
}

type fixCommand func(context.Context, string, string, ...string) ([]byte, error)

type fixReplay struct {
	snapshot verifiedFixSnapshot
	root     string
	config   string
	command  fixCommand
}

func snapshotTree(root string) (fixSnapshot, error) {
	module, openErr := os.OpenRoot(root)
	if openErr != nil {
		return nil, fmt.Errorf("open autofix snapshot root: %w", openErr)
	}
	defer func() { _ = module.Close() }() //nolint:errcheck // closing the read handle cannot affect snapshot contents
	snapshot := fixSnapshot{}
	err := fs.WalkDir(module.FS(), ".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == gitDir || entry.Name() == nodeModulesDir {
				return fs.SkipDir
			}
			return nil
		}
		if !fixSnapshotFile(entry.Name()) {
			return nil
		}
		return snapshot.save(module, path, entry)
	})
	if err != nil {
		return nil, fmt.Errorf("autofix snapshot: %w", err)
	}
	return snapshot, nil
}

func fixSnapshotFile(name string) bool {
	return strings.HasSuffix(name, goExt) || name == fileGoMod || name == fileGoSum ||
		name == fileGoWork || name == fileGoWorkSum
}

func (snapshot fixSnapshot) copyReplay(source, root string) error {
	err := snapshot.restore(root)
	if err != nil {
		return err
	}
	module, err := os.OpenRoot(source)
	if err != nil {
		return fmt.Errorf("open replay source: %w", err)
	}
	defer func() { _ = module.Close() }() //nolint:errcheck // closing the read handle cannot affect replay contents
	for path, saved := range snapshot {
		if !strings.HasSuffix(path, goExt) {
			continue
		}
		patterns, parseErr := replayEmbedPatterns(path, saved.data)
		if parseErr != nil {
			return parseErr
		}
		for _, pattern := range patterns {
			copyErr := snapshot.copyEmbedPattern(module, source, root, filepath.Dir(path), pattern)
			if copyErr != nil {
				return copyErr
			}
		}
	}
	return nil
}

func replayEmbedPatterns(path string, data []byte) ([]string, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, data, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse replay embeds %s: %w", path, err)
	}
	patterns := []string{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			args, found := strings.CutPrefix(comment.Text, embedDirective)
			if !found || args == "" || !unicode.IsSpace(rune(args[0])) {
				continue
			}
			parsed, parseErr := splitEmbedPatterns(args)
			if parseErr != nil {
				return nil, parseErr
			}
			patterns = append(patterns, parsed...)
		}
	}
	return patterns, nil
}

func splitEmbedPatterns(args string) ([]string, error) {
	patterns := []string{}
	for args = strings.TrimSpace(args); args != ""; args = strings.TrimSpace(args) {
		pattern, rest, err := nextEmbedPattern(args)
		if err != nil {
			return nil, err
		}
		patterns = append(patterns, pattern)
		args = rest
	}
	return patterns, nil
}

func nextEmbedPattern(args string) (string, string, error) {
	if args[0] == '"' || args[0] == '`' {
		quoted, err := strconv.QuotedPrefix(args)
		if err != nil {
			return "", "", fmt.Errorf("replay embed pattern: %w", err)
		}
		pattern, unquoteErr := strconv.Unquote(quoted)
		if unquoteErr != nil {
			return "", "", fmt.Errorf("replay embed quote: %w", unquoteErr)
		}
		return pattern, args[len(quoted):], nil
	}
	end := strings.IndexFunc(args, unicode.IsSpace)
	if end < 0 {
		end = len(args)
	}
	return args[:end], args[end:], nil
}

func (snapshot fixSnapshot) copyEmbedPattern(module *os.Root, source, root, dir, pattern string) error {
	pattern = strings.TrimPrefix(pattern, "all:")
	if !fs.ValidPath(pattern) {
		return fmt.Errorf("replay embed pattern %q: %w", pattern, fs.ErrInvalid)
	}
	matches, err := filepath.Glob(filepath.Join(source, dir, pattern))
	if err != nil {
		return fmt.Errorf("replay embed glob: %w", err)
	}
	for _, match := range matches {
		path, relErr := filepath.Rel(source, match)
		if relErr != nil {
			return fmt.Errorf("replay embed path: %w", relErr)
		}
		copyErr := snapshot.copyEmbedTarget(module, root, filepath.ToSlash(path))
		if copyErr != nil {
			return copyErr
		}
	}
	return nil
}

func (snapshot fixSnapshot) copyEmbedTarget(module *os.Root, root, target string) error {
	err := fs.WalkDir(module.FS(), target, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if _, exists := snapshot[path]; exists {
			return nil
		}
		file := fixSnapshot{}
		saveErr := file.save(module, path, entry)
		if saveErr != nil {
			return saveErr
		}
		if saved, exists := file[path]; exists {
			return writeSaved(root, path, saved)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("copy replay embed %s: %w", target, err)
	}
	return nil
}

func (snapshot fixSnapshot) save(module *os.Root, path string, entry fs.DirEntry) error {
	info, infoErr := entry.Info()
	if infoErr != nil {
		return fmt.Errorf("snapshot metadata %s: %w", path, infoErr)
	}
	if !info.Mode().IsRegular() {
		if strings.HasSuffix(path, goExt) {
			return fmt.Errorf("%w: %s", ErrSnapshotSource, path)
		}
		return nil
	}
	data, readErr := module.ReadFile(path)
	if readErr != nil {
		return fmt.Errorf("snapshot read %s: %w", path, readErr)
	}
	snapshot[path] = savedFile{data: data, mode: info.Mode().Perm()}
	return nil
}

func (snapshot fixSnapshot) restore(root string) error {
	current, scanErr := snapshotTree(root)
	var failures []error
	if scanErr != nil {
		failures = append(failures, scanErr)
	}
	for path := range current {
		if _, exists := snapshot[path]; !exists {
			removeErr := os.Remove(filepath.Join(root, path))
			if removeErr != nil {
				failures = append(failures, removeErr)
			}
		}
	}
	for path, saved := range snapshot {
		if now, exists := current[path]; exists && bytes.Equal(now.data, saved.data) && now.mode == saved.mode {
			continue
		}
		writeErr := writeSaved(root, path, saved)
		if writeErr != nil {
			failures = append(failures, writeErr)
		}
	}
	return errors.Join(failures...)
}

func writeSaved(root, path string, saved savedFile) error {
	full := filepath.Join(root, path)
	dirErr := os.MkdirAll(filepath.Dir(full), privateDirMode)
	if dirErr != nil {
		return fmt.Errorf("restore directory %s: %w", path, dirErr)
	}
	writeErr := os.WriteFile(full, saved.data, saved.mode)
	if writeErr != nil {
		return fmt.Errorf("restore %s: %w", path, writeErr)
	}
	chmodErr := os.Chmod(full, saved.mode)
	if chmodErr != nil {
		return fmt.Errorf("restore permissions %s: %w", path, chmodErr)
	}
	return nil
}

func verifyFix(ctx context.Context, root string) error {
	return verifyFixPackages(ctx, root, []string{allPackages}, runFixCommand)
}

func runFixCommand(ctx context.Context, root, program string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext( //nolint:gosec // program comes only from goCmd or bin(golangciBin), never findings
		ctx,
		program,
		args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("autofix command: %w", err)
	}
	return out, nil
}

func verifyFixPackages(ctx context.Context, root string, packages []string, command fixCommand) error {
	if len(packages) == 0 {
		return nil
	}
	for _, action := range []string{"build", cmdVet} {
		args := append([]string{action}, packages...)
		out, err := command(ctx, root, goCmd, args...)
		if err != nil {
			return fmt.Errorf("go %s %s: %w\n%s", action, strings.Join(packages, " "), err, out)
		}
	}
	return nil
}

func (g *gateCtx) safeFix() error {
	baselineErr := verifyFix(g.ctx, ".")
	if baselineErr != nil {
		return fmt.Errorf("%w: autofix refused before writes: %w", ErrGate, baselineErr)
	}
	snapshot, snapshotErr := snapshotTree(".")
	if snapshotErr != nil {
		return snapshotErr
	}
	checkArgs := append(golangciArgs(g.cfg, false, inCI()), "--uniq-by-line=false")
	before := collectGolangci(g.ctx, checkArgs)
	if len(before.notes) > 0 {
		return fmt.Errorf("%w: autofix preflight: %s", ErrGate, strings.Join(before.notes, "\n"))
	}
	_, transformError := transformGate(true)
	if transformError != nil {
		return errors.Join(transformError, snapshot.restore("."))
	}
	fixed := collectGolangci(g.ctx, golangciArgs(g.cfg, true, inCI()))
	validationErr := verifyFix(g.ctx, ".")
	if validationErr != nil {
		restoreErr := snapshot.restore(".")
		proven := verifiedFixSnapshot{files: snapshot}
		culprits := g.offendingFixers(proven, before.diags, validationErr, runFixCommand)
		rollback := "restored the pre-fix snapshot"
		if restoreErr != nil {
			rollback = "rollback incomplete"
		}
		return errors.Join(
			fmt.Errorf("%w: golangci-lint autofix failed (%s); %s:\n%w", ErrGate, culprits, rollback, validationErr),
			restoreErr,
		)
	}
	if len(fixed.notes) > 0 {
		return errors.Join(fmt.Errorf("%w: autofix: %s", ErrGate, strings.Join(fixed.notes, "\n")), snapshot.restore("."))
	}
	return nil
}

func autofixNames(findings []diag.Diagnostic, args []string) []string {
	disabled := []string{}
	for at, arg := range args {
		if arg == disableFlag && at+1 < len(args) {
			disabled = append(disabled, strings.Split(args[at+1], ",")...)
		}
	}
	names := []string{}
	for _, finding := range findings {
		if !slices.Contains(disabled, finding.Linter) && !slices.Contains(names, finding.Linter) {
			names = append(names, finding.Linter)
		}
	}
	slices.Sort(names)
	return names
}

func (g *gateCtx) offendingFixers(
	snapshot verifiedFixSnapshot, findings []diag.Diagnostic, validationErr error, command fixCommand,
) string {
	names := autofixNames(findings, golangciArgs(g.cfg, true, inCI()))
	prioritizeFixers(names, findings, snapshot.files, validationErr)
	fallback := "combined fixes; candidate linters: " + strings.Join(names, ", ")
	root, tempErr := os.MkdirTemp(emptyArg, "lintmax-autofix-replay-")
	if tempErr != nil {
		return fallback
	}
	defer func() { _ = os.RemoveAll(root) }() //nolint:errcheck // replay cleanup cannot affect the restored module
	copyErr := snapshot.files.copyReplay(".", root)
	if copyErr != nil {
		return fallback
	}
	raw, configErr := os.ReadFile(filepath.Clean(g.cfg))
	if configErr != nil {
		return fallback
	}
	replay := fixReplay{snapshot: snapshot, root: root, config: string(raw), command: command}
	bad := []string{}
	incomplete := false
	for _, name := range names {
		broken, replayErr := replay.replayFix(g.ctx, name)
		if replayErr != nil {
			incomplete = true
		}
		if broken {
			bad = append(bad, name)
		}
		restoreErr := snapshot.files.restore(root)
		if restoreErr != nil {
			return fallback
		}
	}
	if len(bad) > 0 && !incomplete {
		slices.Sort(bad)
		return offendingLinters + strings.Join(bad, ", ")
	}
	return fallback
}

func prioritizeFixers(names []string, findings []diag.Diagnostic, snapshot fixSnapshot, validationErr error) {
	if validationErr == nil {
		return
	}
	priority := map[string]bool{}
	root, rootErr := filepath.Abs(".")
	if rootErr != nil {
		return
	}
	for _, finding := range findings {
		path := filepath.Clean(finding.File)
		if filepath.IsAbs(path) {
			rel, relErr := filepath.Rel(root, path)
			if relErr != nil {
				continue
			}
			path = rel
		}
		if _, exists := snapshot[path]; exists && strings.Contains(validationErr.Error(), filepath.ToSlash(path)+":") {
			priority[finding.Linter] = true
		}
	}
	slices.SortStableFunc(names, func(left, right string) int {
		if priority[left] == priority[right] {
			return strings.Compare(left, right)
		}
		if priority[left] {
			return -1
		}
		return 1
	})
}

func (replay fixReplay) replayFix(ctx context.Context, name string) (bool, error) {
	cfg := filepath.Join(replay.root, replayConfigFile)
	raw, linter := replayConfig(replay.config, name)
	writeErr := os.WriteFile(cfg, []byte(raw), configMode)
	if writeErr != nil {
		return false, fmt.Errorf("replay config: %w", writeErr)
	}
	baseArgs := golangciArgs(cfg, false, inCI())
	args := append([]string{baseArgs[0], fixFlag}, baseArgs[1:]...)
	args = append(args, "--enable-only", linter)
	_, _ = replay.command(ctx, replay.root, bin(golangciBin), args...) //nolint:errcheck // findings exit nonzero
	current, scanErr := snapshotTree(replay.root)
	if scanErr != nil {
		return false, scanErr
	}
	packages := changedFixPackages(replay.snapshot.files, current)
	return verifyFixPackages(ctx, replay.root, packages, replay.command) != nil, nil
}

func changedFixPackages(before, after fixSnapshot) []string {
	dirs := map[string]bool{}
	for _, pair := range [][2]fixSnapshot{{before, after}, {after, before}} {
		for path, saved := range pair[0] {
			if !strings.HasSuffix(path, goExt) {
				continue
			}
			now, exists := pair[1][path]
			if !exists || !bytes.Equal(saved.data, now.data) {
				dirs["./"+filepath.ToSlash(filepath.Dir(path))] = true
			}
		}
	}
	packages := make([]string, 0, len(dirs))
	for dir := range dirs {
		packages = append(packages, dir)
	}
	slices.Sort(packages)
	return packages
}

func replayConfig(cfg, name string) (string, string) {
	start := strings.Index(cfg, formattersKey)
	if start < 0 {
		return cfg, name
	}
	end := start + len(formattersKey)
	for end < len(cfg) && (cfg[end] == ' ' || cfg[end] == '\n' || cfg[end] == '#') {
		next := strings.IndexByte(cfg[end:], '\n')
		if next < 0 {
			end = len(cfg)
			break
		}
		end += next + 1
	}
	section := cfg[start:end]
	settings := strings.Index(section, "\n  settings:")
	enabled := section
	if settings >= 0 {
		enabled = section[:settings]
	}
	if strings.Contains(enabled, "    - "+name+"\n") {
		formatter := formattersKey + "  enable:\n    - " + name + "\n"
		if settings >= 0 {
			formatter += section[settings+1:]
		}
		return cfg[:start] + formatter + cfg[end:], "ineffassign"
	}
	return cfg[:start] + formattersKey + "  enable: []\n" + cfg[end:], name
}
