package repo

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	lineWidth   = 123
	indentWidth = 2
	dprintBin   = "dprint"
	typosBin    = "typos"
	dprintFile  = "dprint.json"
	typosFile   = "typos.toml"
	configFlag  = "--config"
	configMode  = 0o600
)

const generatedProbeBytes = 256

const allFilesEnv = "LINTMAX_ALL_FILES"

func minified() []string {
	return []string{"*.min.js", "*.min.css", "*.min.mjs", "*.min.map", "*.lock"}
}

func unauthored() []string {
	return []string{"testdata", "node_modules", "vendor", "dist", "target"}
}

func quoted(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, "\""+value+"\"")
	}
	return out
}

func excludes() []string {
	return append([]string{
		"**/.git",
		"**/node_modules",
		"**/vendor",
		"**/testdata",
		"**/dist",
		"**/target",
		"**/go.sum",
	}, prefixed(minified())...)
}

func prefixed(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, "**/"+value)
	}
	return out
}

func Config(ctx context.Context, dir string) (string, error) {
	body, err := json.MarshalIndent(map[string]any{
		"lineWidth":   lineWidth,
		"indentWidth": indentWidth,
		"useTabs":     false,
		"newLineKind": "lf",
		"includes":    []string{"**/*"},
		"excludes":    excludes(),
		"plugins":     Latest(ctx, Seed()),
	}, "", "  ")
	if err != nil {
		return "", fmt.Errorf("encoding the dprint config: %w", err)
	}
	dprintErr := os.WriteFile(filepath.Join(dir, dprintFile), body, configMode)
	if dprintErr != nil {
		return "", fmt.Errorf("writing the dprint config: %w", dprintErr)
	}
	typos := "[default]\ncheck-filename = true\ncheck-file = true\n" +
		"[files]\nextend-exclude = [" + strings.Join(quoted(append(minified(), unauthored()...)), ", ") + "]\n"
	typosErr := os.WriteFile(filepath.Join(dir, typosFile), []byte(typos), configMode)
	if typosErr != nil {
		return "", fmt.Errorf("writing the typos config: %w", typosErr)
	}
	return dir, nil
}

// A project may supply the DATA a speller needs to be correct about it — its own domain vocabulary —
// through the tool's OWN config file rather than a lintmax-specific one, so there is no second home
// for a fact `typos` already owns. It supplies data only: no rule can be turned off from there,
// because the file replaces the word list rather than the check.
func typosConfig(root, cfg string) string {
	own := filepath.Join(root, typosFile)
	_, err := os.Stat(own)
	if err != nil {
		return filepath.Join(cfg, typosFile)
	}
	return own
}

// A GENERATED ARTIFACT IS NOT AUTHORED PROSE, and a formatter that cannot parse one fails the whole
// stage rather than skipping it — measured on a Tailwind stylesheet whose own banner says it is
// generated. The marker is the file's own first line, which is a convention every generator already
// follows, so nothing has to be listed by hand and a new artifact is covered the day it appears.
func generated(root string) []string {
	var found []string
	_ = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error { //nolint:errcheck // reason: a tree we cannot walk simply contributes no exclusions
		if err != nil || entry.IsDir() {
			return nil //nolint:nilerr // reason: an unreadable entry is skipped rather than failing the gate
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".css", ".js", ".mjs", ".ts":
		default:
			return nil
		}
		handle, openErr := os.Open(path)
		if openErr != nil {
			return nil
		}
		defer func() { _ = handle.Close() }() //nolint:errcheck // reason: read-only probe
		head := make([]byte, generatedProbeBytes)
		read, _ := handle.Read(head) //nolint:errcheck // reason: a short read still carries the banner
		first := string(head[:read])
		if line, _, cut := strings.Cut(first, "\n"); cut || line != "" {
			lower := strings.ToLower(line)
			for _, marker := range []string{"/*!", "code generated", "do not edit", "@generated"} {
				if strings.Contains(lower, marker) {
					relative, relErr := filepath.Rel(root, path)
					if relErr == nil {
						found = append(found, relative)
					}
					break
				}
			}
		}
		return nil
	})
	return found
}

// dprint takes ONE `--excludes` carrying every pattern; repeating the flag is refused outright with
// `cannot be used multiple times`, which fails the stage rather than excluding anything.
func dprintArgs(action, cfg string, skip []string) []string {
	args := []string{action, configFlag, cfg}
	if len(skip) == 0 {
		return args
	}
	return append(append(args, "--excludes"), skip...)
}

// THE WHOLE-TREE STAGES ARE OPT-IN UNTIL A REPOSITORY HAS GROUND ITS BASELINE, because a gate that
// starts judging every non-Go file the day it is released BLOCKS EVERY COMMIT on findings nobody in
// that repository introduced — measured on one consumer at 2,187 shellcheck findings and 170 files of
// formatter churn, none of them from the change under test. The capability is unchanged and the
// consumer turns it on when its baseline is clear, which is the ratchet rather than a cut: a repo
// that never enables it keeps exactly the gate it shipped with.
func WholeTreeRequested() bool {
	return strings.TrimSpace(os.Getenv(allFilesEnv)) != ""
}

func Gate(ctx context.Context, root string, fix bool) []string {
	if !WholeTreeRequested() {
		return nil
	}
	dir, err := os.MkdirTemp("", "lintmax-go-repo-")
	if err != nil {
		return []string{"repo config: " + err.Error()}
	}
	defer func() { _ = os.RemoveAll(dir) }() //nolint:errcheck // a temp dir that outlives the run is harmless
	cfg, cfgErr := Config(ctx, dir)
	if cfgErr != nil {
		return []string{"repo config: " + cfgErr.Error()}
	}
	action := "check"
	if fix {
		action = "fmt"
	}
	specs := []struct {
		name string
		args []string
	}{
		{name: dprintBin, args: dprintArgs(action, filepath.Join(cfg, dprintFile), generated(root))},
		{name: typosBin, args: []string{configFlag, typosConfig(root, cfg), root}},
	}
	var notes []string
	for _, spec := range specs {
		notes = append(notes, judge(ctx, root, spec.name, spec.args)...)
	}
	return notes
}

func judge(ctx context.Context, root, name string, args []string) []string {
	_, lookErr := exec.LookPath(name)
	if lookErr != nil {
		return []string{name + ": not installed, so every non-Go file went unchecked"}
	}
	//nolint:gosec // fixed tool name, and every argument is generated by this package
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = root
	out, runErr := cmd.CombinedOutput()
	if runErr == nil {
		return nil
	}
	return []string{name + ":\n" + strings.TrimRight(string(out), "\n")}
}
