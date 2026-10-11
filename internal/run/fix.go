package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"github.com/1qh/lintmax-go/internal/diag"
)

const (
	privateDirMode = 0o700
	formattersKey  = "\nformatters:\n"
)

var ErrSnapshotSource = errors.New("cannot snapshot non-regular Go source")

type savedFile struct {
	data []byte
	mode fs.FileMode
}

type fixSnapshot map[string]savedFile

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
		return snapshot.save(module, path, entry)
	})
	if err != nil {
		return nil, fmt.Errorf("autofix snapshot: %w", err)
	}
	return snapshot, nil
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
	for _, action := range []string{"build", cmdVet} {
		cmd := exec.CommandContext(ctx, goCmd, action, allPackages) //nolint:gosec // fixed Go validation commands
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("go %s %s: %w\n%s", action, allPackages, err, out)
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
		culprits := g.offendingFixers(snapshot, before.diags)
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

func (g *gateCtx) offendingFixers(snapshot fixSnapshot, findings []diag.Diagnostic) string {
	names := autofixNames(findings, golangciArgs(g.cfg, true, inCI()))
	bad := []string{}
	for _, name := range names {
		if g.replayFix(snapshot, name) {
			bad = append(bad, name)
		}
	}
	if len(bad) > 0 {
		return "offending linters: " + strings.Join(bad, ", ")
	}
	return "combined fixes; candidate linters: " + strings.Join(names, ", ")
}

func (g *gateCtx) replayFix(snapshot fixSnapshot, name string) bool {
	root, tempErr := os.MkdirTemp(emptyArg, "lintmax-autofix-replay-")
	if tempErr != nil {
		return false
	}
	defer func() { _ = os.RemoveAll(root) }() //nolint:errcheck // replay cleanup cannot affect the restored module
	for path, saved := range snapshot {
		writeErr := writeSaved(root, path, saved)
		if writeErr != nil {
			return false
		}
	}
	baselineErr := verifyFix(g.ctx, root)
	if baselineErr != nil {
		return false
	}
	raw, configErr := os.ReadFile(filepath.Clean(g.cfg))
	if configErr != nil {
		return false
	}
	cfg := filepath.Join(root, ".lintmax-replay.yml")
	replay, linter := replayConfig(string(raw), name)
	writeErr := os.WriteFile(cfg, []byte(replay), configMode) //nolint:gosec // cfg is in the isolated replay
	if writeErr != nil {
		return false
	}
	args := golangciArgs(cfg, false, inCI())
	args = append(args, fixFlag, "--enable-only", linter)
	cmd := exec.CommandContext(g.ctx, bin(golangciBin), args...) //nolint:gosec // replay a linter in an isolated snapshot
	cmd.Dir = root
	_ = cmd.Run() //nolint:errcheck // findings exit nonzero; compilation determines whether the fix is broken
	return verifyFix(g.ctx, root) != nil
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
