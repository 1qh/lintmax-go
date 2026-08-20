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

func typosConfig(root, cfg string) string {
	own := filepath.Join(root, typosFile)
	_, err := os.Stat(own)
	if err != nil {
		return filepath.Join(cfg, typosFile)
	}
	return own
}

func generated(root string) []string {
	found := make([]string, 0, generatedProbeBytes)
	walkErr := filepath.WalkDir(
		root,
		func(path string, entry fs.DirEntry, err error) error { //nolint:errcheck // reason: a tree we cannot walk simply contributes no exclusions
			if relative, ok := generatedRelative(root, path, entry, err); ok {
				found = append(found, relative)
			}
			return nil
		},
	)
	if walkErr != nil {
		return found
	}
	return found
}

func generatedRelative(root, path string, entry fs.DirEntry, walkErr error) (string, bool) {
	if walkErr != nil || entry.IsDir() {
		return "", false
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".css", ".js", ".mjs", ".ts":
	default:
		return "", false
	}
	//nolint:gosec // reason: path comes from walking the repository selected by the caller
	handle, openErr := os.Open(path)
	if openErr != nil {
		return "", false
	}
	defer func() { _ = handle.Close() }() //nolint:errcheck // reason: read-only probe
	head := make([]byte, generatedProbeBytes)
	read, _ := handle.Read(head) //nolint:errcheck // reason: a short read still carries the banner
	line, _, cut := strings.Cut(string(head[:read]), "\n")
	if !cut && line == "" || !generatedBanner(line) {
		return "", false
	}
	relative, relErr := filepath.Rel(root, path)
	return relative, relErr == nil
}

func generatedBanner(line string) bool {
	lower := strings.ToLower(line)
	for _, marker := range []string{"/*!", "code generated", "do not edit", "@generated"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func dprintArgs(action, cfg string, skip []string) []string {
	args := make([]string, 3, 4+len(skip))
	args[0], args[1], args[2] = action, configFlag, cfg
	if len(skip) == 0 {
		return args
	}
	return append(append(args, "--excludes"), skip...)
}

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
