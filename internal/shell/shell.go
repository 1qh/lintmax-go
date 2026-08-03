package shell

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	shfmtBin      = "shfmt"
	shellcheckBin = "shellcheck"
)

func ShfmtFlags() []string { return []string{"-s", "-ci", "-bn", "-sr", "-i", "2"} }

func ShellcheckFlags() []string {
	return []string{"--enable=all", "--severity=style", "--external-sources"}
}

func skipped(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", "testdata":
		return true
	default:
		return false
	}
}

func Scripts(dir string) ([]string, error) {
	var found []string
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if skipped(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".sh") {
			found = append(found, path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking %s for shell scripts: %w", dir, err)
	}
	return found, nil
}

func Gate(ctx context.Context, dir string, fix bool) []string {
	scripts, err := Scripts(dir)
	if err != nil {
		return []string{"shell scan: " + err.Error()}
	}
	if len(scripts) == 0 {
		return nil
	}
	write := "-d"
	if fix {
		write = "-w"
	}
	specs := []struct {
		name string
		args []string
	}{
		{name: shfmtBin, args: append(append([]string{write}, ShfmtFlags()...), scripts...)},
		{name: shellcheckBin, args: append(ShellcheckFlags(), scripts...)},
	}
	var notes []string
	for _, spec := range specs {
		notes = append(notes, judge(ctx, spec.name, spec.args, len(scripts))...)
	}
	return notes
}

func judge(ctx context.Context, name string, args []string, scripts int) []string {
	_, lookErr := exec.LookPath(name)
	if lookErr != nil {
		return []string{name + ": not installed, so " + plural(scripts) + " went unchecked"}
	}
	//nolint:gosec // fixed tool name, and the file list is derived from the tree rather than input
	cmd := exec.CommandContext(ctx, name, args...)
	out, runErr := cmd.CombinedOutput()
	if runErr == nil && len(out) == 0 {
		return nil
	}
	return []string{name + ":\n" + strings.TrimRight(string(out), "\n")}
}

func plural(count int) string {
	if count == 1 {
		return "1 shell script"
	}
	return strconv.Itoa(count) + " shell scripts"
}
