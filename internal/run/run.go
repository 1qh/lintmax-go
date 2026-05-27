package run

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/1qh/lintmax-go/internal/config"
	"github.com/1qh/lintmax-go/internal/diag"
	"github.com/1qh/lintmax-go/internal/staleness"
	"github.com/1qh/lintmax-go/internal/state"
	"github.com/1qh/lintmax-go/internal/tools"
	"github.com/1qh/lintmax-go/internal/transform"
	"github.com/1qh/lintmax-go/internal/version"
)

const (
	emptyArg   = ""
	goCmd      = "go"
	configMode = 0o600
	dirPerm    = 0o755
	refreshTTL = 24 * time.Hour
)

var ErrGate = errors.New("gate failed")

func binDir() string {
	if v := os.Getenv("GOBIN"); v != emptyArg {
		return v
	}
	if v := os.Getenv("GOPATH"); v != emptyArg {
		return filepath.Join(v, "bin")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = emptyArg
	}
	return filepath.Join(home, goCmd, "bin")
}

func bin(name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(binDir(), name)
}

func toolVersion(ctx context.Context, binPath string) string {
	out, err := exec.CommandContext(ctx, goCmd, "version", "-m", binPath).Output() //nolint:gosec // fixed binary path
	if err != nil {
		return emptyArg
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 3 && fields[0] == "mod" {
			return fields[2]
		}
	}
	return emptyArg
}

func reportBumps(ctx context.Context, installed []tools.Tool) {
	prev := state.Load()
	next := state.State{Versions: map[string]string{}, LastCheck: time.Time{}}
	for _, tool := range installed {
		ver := toolVersion(ctx, bin(tool.Name))
		next.Versions[tool.Name] = ver
		old := prev.Versions[tool.Name]
		if old != emptyArg && old != ver {
			fmt.Fprintf(os.Stderr, "↑ %s %s → %s\n", tool.Name, old, ver)
		}
	}
	next.LastCheck = time.Now()
	saveErr := next.Save()
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go: version cache:", saveErr)
	}
}

func toolsPresent(includeDeep bool) bool {
	for _, tool := range tools.All {
		if tool.Deep && !includeDeep {
			continue
		}
		_, err := os.Stat(bin(tool.Name))
		if err != nil {
			return false
		}
	}
	return true
}

func EnsureLatest(ctx context.Context, includeDeep, force bool) error {
	if !force && state.Load().Fresh(refreshTTL) && toolsPresent(includeDeep) {
		return nil
	}
	var installed []tools.Tool
	for _, tool := range tools.All {
		if tool.Deep && !includeDeep {
			continue
		}
		cmd := exec.CommandContext(ctx, goCmd, "install", tool.Pkg+"@latest") //nolint:gosec // static registry paths
		var buf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &buf, &buf
		err := cmd.Run()
		if err != nil {
			fmt.Fprint(os.Stderr, buf.String())
			return fmt.Errorf("install %s: %w", tool.Name, err)
		}
		installed = append(installed, tool)
	}
	reportBumps(ctx, installed)
	return nil
}

func writeConfig() (string, error) {
	dir, err := os.MkdirTemp(emptyArg, "lintmax-go")
	if err != nil {
		return emptyArg, fmt.Errorf("temp dir: %w", err)
	}
	path := filepath.Join(dir, ".golangci.yml")
	err = os.WriteFile(path, config.GolangCI, configMode)
	if err != nil {
		return emptyArg, fmt.Errorf("write config: %w", err)
	}
	return path, nil
}

func runOut(ctx context.Context, name string, args ...string) ([]byte, bool) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed tool invocations
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	return out.Bytes(), err == nil
}

func runCombined(ctx context.Context, name string, args ...string) ([]byte, bool) {
	cmd := exec.CommandContext(ctx, name, args...) //nolint:gosec // fixed tool invocations
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	return buf.Bytes(), err == nil
}

func tailLines(data []byte, count int) string {
	lines := strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	if len(lines) > count {
		lines = lines[len(lines)-count:]
	}
	return strings.Join(lines, "\n")
}

func Upgrade(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, goCmd, "install", version.Self+"@latest") //nolint:gosec // fixed self path
	var buf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &buf, &buf
	err := cmd.Run()
	if err != nil {
		fmt.Fprint(os.Stderr, buf.String())
		return fmt.Errorf("upgrade: %w", err)
	}
	return nil
}

type scaffold struct {
	path string
	data []byte
}

func writeIfAbsent(item scaffold) (bool, error) {
	_, err := os.Stat(item.path)
	if err == nil {
		return false, nil
	}
	mkErr := os.MkdirAll(filepath.Dir(item.path), dirPerm)
	if mkErr != nil {
		return false, fmt.Errorf("mkdir %s: %w", item.path, mkErr)
	}
	wErr := os.WriteFile(item.path, item.data, configMode)
	if wErr != nil {
		return false, fmt.Errorf("write %s: %w", item.path, wErr)
	}
	return true, nil
}

func Init() error {
	items := []scaffold{
		{path: ".editorconfig", data: config.EditorConfig},
		{path: filepath.Join(".github", "workflows", "ci.yml"), data: config.ConsumerCI},
	}
	for _, item := range items {
		wrote, err := writeIfAbsent(item)
		if err != nil {
			return err
		}
		status := "exists "
		if wrote {
			status = "created"
		}
		fmt.Fprintf(os.Stderr, "%s %s\n", status, item.path)
	}
	return nil
}

func Rules(ctx context.Context) error {
	cfg, err := writeConfig()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, bin("golangci-lint"), "linters", "--config", cfg) //nolint:gosec // fixed invocation
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	runErr := cmd.Run()
	if runErr != nil {
		return fmt.Errorf("rules: %w", runErr)
	}
	return nil
}

func transformGate(fix bool) ([]string, error) {
	if fix {
		_, err := transform.Apply(".")
		if err != nil {
			return nil, fmt.Errorf("transform: %w", err)
		}
		return nil, nil
	}
	changed, err := transform.Check(".")
	if err != nil {
		return nil, fmt.Errorf("transform: %w", err)
	}
	return changed, nil
}

func deepScan(ctx context.Context) []string {
	var notes []string
	specs := []struct {
		name string
		args []string
	}{
		{name: "govulncheck", args: []string{"./..."}},
		{name: "osv-scanner", args: []string{"scan", "source", "-r", "."}},
		{name: "capslock", args: []string{"-packages", "./..."}},
	}
	for _, spec := range specs {
		out, ok := runCombined(ctx, bin(spec.name), spec.args...)
		if !ok {
			notes = append(notes, spec.name+":\n"+tailLines(out, 15))
		}
	}
	return notes
}

func collect(ctx context.Context, cfg string, fix bool) ([]diag.Diagnostic, []string) {
	var diags []diag.Diagnostic
	var notes []string
	gcArgs := []string{"run", "--config", cfg, "--output.json.path=stdout", "--output.text.path=" + os.DevNull}
	if fix {
		gcArgs = append(gcArgs, "--fix")
	}
	gcOut, gcOK := runOut(ctx, bin("golangci-lint"), gcArgs...)
	gcDiags := diag.ParseGolangci(gcOut)
	diags = append(diags, gcDiags...)
	if !gcOK && len(gcDiags) == 0 {
		notes = append(notes, "golangci-lint:\n"+tailLines(gcOut, 15))
	}
	nilOut, _ := runOut(ctx, bin("nilaway"), "-json", "./...")
	diags = append(diags, diag.ParseAnalysis(nilOut, "nilaway")...)
	dcOut, dcOK := runCombined(ctx, bin("deadcode"), "-test", "./...")
	if !dcOK {
		diags = append(diags, diag.ParseLines(dcOut, "deadcode")...)
	}
	return diags, notes
}

func report(diags []diag.Diagnostic, notes []string) error {
	root, gwErr := os.Getwd()
	if gwErr != nil {
		root = "."
	}
	out := diag.Format(diags, root)
	if out == "" && len(notes) == 0 {
		fmt.Fprintln(os.Stdout, "ok")
		return nil
	}
	fmt.Fprint(os.Stderr, out)
	for _, note := range notes {
		fmt.Fprintln(os.Stderr, note)
	}
	return fmt.Errorf("%w: %d", ErrGate, diag.Count(diags)+len(notes))
}

func Gate(ctx context.Context, fix, deep bool) error {
	cfg, err := writeConfig()
	if err != nil {
		return err
	}
	changed, terr := transformGate(fix)
	if terr != nil {
		return terr
	}
	diags, notes := collect(ctx, cfg, fix)
	if len(changed) > 0 {
		notes = append(notes, "comments/blanks (run fix): "+strings.Join(changed, ", "))
	}
	testOut, testOK := runCombined(ctx, goCmd, "test", "-race", "-shuffle=on", "./...")
	if !testOK {
		notes = append(notes, "go test:\n"+tailLines(testOut, 20))
	}
	stale, sErr := staleness.Scan(ctx, ".")
	if sErr != nil {
		notes = append(notes, "staleness scan: "+sErr.Error())
	}
	if rendered := staleness.Format(stale); rendered != "" {
		notes = append(notes, fmt.Sprintf("%d dep(s) stale (bump or pin):\n%s", len(stale), rendered))
	}
	if deep {
		notes = append(notes, deepScan(ctx)...)
	}
	return report(diags, notes)
}
