package run

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/1qh/lintmax-go/internal/config"
	"github.com/1qh/lintmax-go/internal/diag"
	"github.com/1qh/lintmax-go/internal/dupconst"
	"github.com/1qh/lintmax-go/internal/floatdiv"
	"github.com/1qh/lintmax-go/internal/idiom"
	"github.com/1qh/lintmax-go/internal/override"
	"github.com/1qh/lintmax-go/internal/staleness"
	"github.com/1qh/lintmax-go/internal/state"
	"github.com/1qh/lintmax-go/internal/tools"
	"github.com/1qh/lintmax-go/internal/transform"
	"github.com/1qh/lintmax-go/internal/version"
)

const (
	emptyArg     = ""
	goCmd        = "go"
	configMode   = 0o600
	dirPerm      = 0o755
	refreshTTL   = 24 * time.Hour
	cmdTest      = "test"
	binSubdir    = "bin"
	cmdInstall   = "install"
	golangciBin  = "golangci-lint"
	allPackages  = "./..."
	transformErr = "transform: %w"
	phaseFmt     = "  %-12s %v\n"
	tailDefault  = 15
	tailTest     = 20
	minParallel  = 2
	cfgFlag      = "--config"
	binDeadcode  = "deadcode"
	binNilaway   = "nilaway"
)

var ErrGate = errors.New("gate failed")

func binDir() string {
	if v := os.Getenv("GOBIN"); v != emptyArg { //nolint:forbidigo // reason: bootstrap layer owns env reads
		return v
	}
	if v := os.Getenv("GOPATH"); v != emptyArg { //nolint:forbidigo // reason: bootstrap layer owns env reads
		return filepath.Join(v, binSubdir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = emptyArg
	}
	return filepath.Join(home, goCmd, binSubdir)
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
	next.LastCheck = time.Now() //nolint:forbidigo // reason: bootstrap layer owns clock reads
	saveErr := next.Save()
	if saveErr != nil {
		fmt.Fprintln(os.Stderr, "lintmax-go: version cache:", saveErr)
	}
}

func toolsPresent() bool {
	for _, tool := range tools.All {
		_, err := os.Stat(bin(tool.Name))
		if err != nil {
			return false
		}
	}
	return true
}

func EnsureLatest(ctx context.Context, force bool) error {
	if !force && !inCI() && state.Load().Fresh(refreshTTL) && toolsPresent() {
		return nil
	}
	var (
		mu        sync.Mutex
		installed []tools.Tool
		firstErr  error
	)
	var wg sync.WaitGroup
	for _, tool := range tools.All {
		wg.Go(func() {
			cmd := exec.CommandContext(ctx, goCmd, cmdInstall, tool.Pkg+"@latest") //nolint:gosec // static registry paths
			var buf bytes.Buffer
			cmd.Stdout, cmd.Stderr = &buf, &buf
			err := cmd.Run()
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				if firstErr == nil {
					firstErr = fmt.Errorf("install %s: %w", tool.Name, err)
					fmt.Fprint(os.Stderr, buf.String())
				}
				return
			}
			installed = append(installed, tool)
		})
	}
	wg.Wait()
	if firstErr != nil {
		return firstErr
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
	cfg := string(config.GolangCI)
	if extra := generatedExclusions(); extra != "" {
		cfg = strings.Replace(cfg, "    paths:\n", "    paths:\n"+extra, 1)
	}
	spec, ovErr := override.Load(".")
	if ovErr != nil {
		return emptyArg, fmt.Errorf("override: %w", ovErr)
	}
	cfg = override.Apply(cfg, spec)
	err = os.WriteFile(path, []byte(cfg), configMode)
	if err != nil {
		return emptyArg, fmt.Errorf("write config: %w", err)
	}
	return path, nil
}

var genHeaderRe = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

const (
	genHeaderScanLines = 10
	goExt              = ".go"
)

func isGenerated(path string) bool {
	f, err := os.Open(path) //nolint:gosec // path from WalkDir over the linted repo
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }() //nolint:errcheck // close in defer on a read-only file
	sc := bufio.NewScanner(f)
	for i := 0; i < genHeaderScanLines && sc.Scan(); i++ {
		if genHeaderRe.MatchString(sc.Text()) {
			return true
		}
	}
	return false
}

func generatedExclusions() string {
	var b strings.Builder
	walk := func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip unreadable entries, keep walking
		}
		if d.IsDir() || !strings.HasSuffix(p, goExt) {
			return nil
		}
		if isGenerated(p) {
			b.WriteString("      - '(^|/)" + regexp.QuoteMeta(strings.TrimPrefix(p, "./")) + "$'\n")
		}
		return nil
	}
	err := filepath.WalkDir(".", walk)
	if err != nil {
		return b.String()
	}
	return b.String()
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
	cmd := exec.CommandContext(ctx, goCmd, cmdInstall, version.Self+"@latest") //nolint:gosec // fixed self path
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
	cmd := exec.CommandContext(ctx, bin(golangciBin), "linters", cfgFlag, cfg) //nolint:gosec // fixed invocation
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
			return nil, fmt.Errorf(transformErr, err)
		}
		return nil, nil
	}
	changed, err := transform.Check(".")
	if err != nil {
		return nil, fmt.Errorf(transformErr, err)
	}
	return changed, nil
}

func deepScan(ctx context.Context) []string {
	specs := []struct {
		name string
		args []string
	}{
		{name: "govulncheck", args: []string{allPackages}},
		{name: "osv-scanner", args: []string{"scan", "source", "-r", "."}},
		{name: "capslock", args: []string{"-packages", "./..."}},
	}
	results := make([]string, len(specs))
	var wg sync.WaitGroup
	for idx, spec := range specs {
		wg.Go(func() {
			out, ok := runCombined(ctx, bin(spec.name), spec.args...)
			if !ok {
				results[idx] = spec.name + ":\n" + tailLines(out, tailDefault)
			}
		})
	}
	wg.Wait()
	var notes []string
	for _, r := range results {
		if r != "" {
			notes = append(notes, r)
		}
	}
	return notes
}

type collectResult struct {
	diags []diag.Diagnostic
	notes []string
}

func inCI() bool {
	return os.Getenv("GITHUB_ACTIONS") != emptyArg || os.Getenv("CI") != emptyArg //nolint:forbidigo // bootstrap env read
}

func isolateCICache() {
	if !inCI() {
		return
	}
	dir, err := os.MkdirTemp(emptyArg, "lintmax-gocache")
	if err != nil {
		return
	}
	_ = os.Setenv("GOLANGCI_LINT_CACHE", dir) //nolint:errcheck,forbidigo // bootstrap cache isolation
}

func collect(ctx context.Context, cfg string, fix bool) ([]diag.Diagnostic, []string) {
	isolateCICache()
	gcArgs := []string{
		"run", cfgFlag, cfg,
		"--concurrency=" + strconv.Itoa(linterConcurrency()),
		"--output.json.path=stdout", "--output.text.path=" + os.DevNull,
	}
	if fix {
		gcArgs = append(gcArgs, "--fix")
	}
	results := make(chan collectResult, 3) //nolint:mnd // golangci + deadcode + nilaway, all always-on for security
	var wg sync.WaitGroup
	wg.Go(func() {
		gcOut, gcOK := runOut(ctx, bin(golangciBin), gcArgs...)
		gcDiags := diag.ParseGolangci(gcOut)
		out := collectResult{diags: gcDiags}
		if !gcOK && len(gcDiags) == 0 {
			out.notes = []string{"golangci-lint:\n" + tailLines(gcOut, tailDefault)}
		}
		results <- out
	})
	wg.Go(func() {
		dcOut, _ := runCombined(ctx, bin(binDeadcode), "-test", allPackages)
		results <- collectResult{diags: diag.ParseLines(dcOut, binDeadcode)}
	})
	wg.Go(func() {
		nilOut, _ := runOut(ctx, bin(binNilaway), "-json", allPackages)
		results <- collectResult{diags: diag.ParseAnalysis(nilOut, binNilaway)}
	})
	wg.Wait()
	close(results)
	var diags []diag.Diagnostic
	var notes []string
	for r := range results {
		diags = append(diags, r.diags...)
		notes = append(notes, r.notes...)
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

type gateCtx struct {
	ctx       context.Context //nolint:containedctx // gate stage struct intentionally bundles ctx
	timing    bool
	fix       bool
	cfg       string
	skipCheck bool
	greenKey  string
}

func Gate(ctx context.Context, fix bool) error {
	timing := os.Getenv("LINTMAX_TIMING") == "1"  //nolint:forbidigo // reason: bootstrap layer owns env reads
	noSkip := os.Getenv("LINTMAX_NO_SKIP") == "1" //nolint:forbidigo // reason: bootstrap layer owns env reads
	g := &gateCtx{
		ctx:       ctx,
		timing:    timing,
		fix:       fix,
		skipCheck: !fix && !noSkip,
	}
	g.greenKey = timePhase3(g.timing, "treehash", computeTreeHash)
	if g.tryCached() {
		return nil
	}
	cfg, err := timePhase(g.timing, "writeConfig", writeConfig)
	if err != nil {
		return err
	}
	g.cfg = cfg
	return g.runGate()
}

func (g *gateCtx) tryCached() bool {
	if !g.skipCheck || g.greenKey == "" {
		return false
	}
	prev := state.Load()
	cwd, cwErr := os.Getwd()
	if cwErr != nil || cwd == "" {
		return false
	}
	if prev.LastGreenByCWD[cwd] != g.greenKey {
		return false
	}
	fmt.Fprintln(os.Stdout, "ok (cached)")
	return true
}

func (g *gateCtx) runGate() error {
	changed, terr := timePhase(g.timing, "transform", func() ([]string, error) { return transformGate(g.fix) })
	if terr != nil {
		return terr
	}
	diags, notes := g.runParallel()
	if len(changed) > 0 {
		notes = append(notes, "comments/blanks (run fix): "+strings.Join(changed, ", "))
	}
	err := report(diags, notes)
	if err == nil && g.skipCheck && g.greenKey != "" {
		persistGreen(g.greenKey)
	}
	return err
}

func (g *gateCtx) runParallel() ([]diag.Diagnostic, []string) {
	var diags []diag.Diagnostic
	var notes []string
	var stale []staleness.Issue
	var staleErr error
	var deepNotes []string
	var testOut []byte
	testOK := true
	skipTest := os.Getenv("LINTMAX_SKIP_TEST") == "1" //nolint:forbidigo // reason: bootstrap layer owns env reads
	var topWG sync.WaitGroup
	topWG.Go(func() {
		diags, notes = timePhase2(g.timing, "collect", func() ([]diag.Diagnostic, []string) {
			return collect(g.ctx, g.cfg, g.fix)
		})
	})
	topWG.Go(func() {
		stale, staleErr = timePhase(g.timing, "staleness", func() ([]staleness.Issue, error) {
			return staleness.Scan(g.ctx, ".")
		})
	})
	topWG.Go(func() {
		deepNotes = timePhase3(g.timing, "vulnScan", func() []string { return deepScan(g.ctx) })
	})
	var dupIssues []dupconst.Issue
	var dupErr error
	topWG.Go(func() {
		dupIssues, dupErr = timePhase(g.timing, "dupconst", func() ([]dupconst.Issue, error) {
			return dupconst.Scan(g.ctx, ".")
		})
	})
	var fdivIssues []floatdiv.Issue
	var fdivErr error
	topWG.Go(func() {
		fdivIssues, fdivErr = timePhase(g.timing, "floatdiv", func() ([]floatdiv.Issue, error) {
			return floatdiv.Scan(g.ctx, ".")
		})
	})
	var idiomIssues []idiom.Issue
	var idiomErr error
	topWG.Go(func() {
		idiomIssues, idiomErr = timePhase(g.timing, "idiom", func() ([]idiom.Issue, error) {
			return idiom.Scan(g.ctx, ".")
		})
	})
	var scriptIssues []string
	topWG.Go(func() {
		issues, serr := idiom.ScanScripts(g.ctx, ".")
		if serr == nil {
			scriptIssues = issues
		}
	})
	if !skipTest {
		topWG.Go(func() {
			testOut, testOK = timePhase2(g.timing, cmdTest, func() ([]byte, bool) {
				return runCombined(g.ctx, goCmd, testArgs()...)
			})
		})
	}
	topWG.Wait()
	if !testOK {
		notes = append(notes, "go test:\n"+tailLines(testOut, tailTest))
	}
	notes = append(notes, depNotes(stale, staleErr, dupErr, dupIssues)...)
	notes = append(notes, fdivNotes(fdivErr, fdivIssues)...)
	notes = append(notes, idiomNotes(idiomErr, idiomIssues)...)
	if r := idiom.FormatScripts(scriptIssues); r != "" {
		notes = append(notes, r)
	}
	notes = append(notes, deepNotes...)
	return diags, notes
}

func depNotes(stale []staleness.Issue, staleErr, dupErr error, dupIssues []dupconst.Issue) []string {
	var notes []string
	if staleErr != nil {
		notes = append(notes, "staleness scan: "+staleErr.Error())
	}
	if dupErr != nil {
		notes = append(notes, "dupconst scan: "+dupErr.Error())
	}
	if direct := staleNote(stale); direct != "" {
		notes = append(notes, direct)
	}
	if rendered := dupconst.Format(dupIssues); rendered != "" {
		fmt.Fprintf(os.Stderr, "advisory: %d duplicate-value const group(s) (collapse to one):\n%s\n",
			len(dupIssues), rendered)
	}
	return notes
}

func fdivNotes(fdivErr error, fdivIssues []floatdiv.Issue) []string {
	var notes []string
	if fdivErr != nil {
		notes = append(notes, "floatdiv scan: "+fdivErr.Error())
	}
	if rendered := floatdiv.Format(fdivIssues); rendered != "" {
		fmt.Fprintf(os.Stderr, "advisory: %d unguarded float-division site(s) (NaN/Inf risk on empty input):\n%s\n",
			len(fdivIssues), rendered)
	}
	return notes
}

func staleNote(stale []staleness.Issue) string {
	var direct, indirect []staleness.Issue
	for _, s := range stale {
		if strings.Contains(s.Source, "indirect") {
			indirect = append(indirect, s)
		} else {
			direct = append(direct, s)
		}
	}
	if len(indirect) > 0 {
		fmt.Fprintf(os.Stderr,
			"advisory: %d indirect dep(s) behind latest (MVS-capped — bump the requiring dep to move):\n%s\n",
			len(indirect), staleness.Format(indirect))
	}
	if len(direct) > 0 {
		return fmt.Sprintf("%d direct dep(s) stale (bump or pin):\n%s", len(direct), staleness.Format(direct))
	}
	return ""
}

func testArgs() []string {
	noRace := os.Getenv("LINTMAX_NO_RACE") == "1" //nolint:forbidigo // reason: bootstrap layer owns env reads
	args := []string{cmdTest, "-p=" + strconv.Itoa(testConcurrency(noRace)), "-shuffle=on", "-vet=off"}
	if !noRace {
		args = append(args, "-race")
	}
	return append(args, allPackages)
}

func linterConcurrency() int {
	n := runtime.NumCPU() / 2 //nolint:mnd // split host CPUs with the parallel test phase
	if n < minParallel {
		return minParallel
	}
	return n
}

func testConcurrency(noRace bool) int {
	if noRace {
		return runtime.NumCPU()
	}
	n := runtime.NumCPU() / 2 //nolint:mnd // split host CPUs with the parallel collect phase
	if n < minParallel {
		return minParallel
	}
	return n
}

func persistGreen(key string) {
	cwd, cwErr := os.Getwd()
	if cwErr != nil {
		return
	}
	st := state.Load()
	st.LastGreenByCWD[cwd] = key
	_ = st.Save() //nolint:errcheck // best-effort cache
}

func computeTreeHash() string {
	h := sha256.New()
	hashWrite(h, "lintmax-go:"+version.Current()+"\n")
	walkErr := filepath.Walk(".", treeWalker(h))
	if walkErr != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func treeWalker(h hash.Hash) filepath.WalkFunc {
	return func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // best-effort walk; unreadable files skipped
		}
		if info.IsDir() {
			return skipNoise(info.Name())
		}
		if !relevantHashFile(path) {
			return nil
		}
		body, rErr := os.ReadFile(path) //nolint:gosec // local repo walk
		if rErr != nil {
			return nil //nolint:nilerr // skip unreadable
		}
		digest := sha256.Sum256(body)
		hashWrite(h, path+":"+hex.EncodeToString(digest[:])+"\n")
		return nil
	}
}

func skipNoise(name string) error {
	if name == ".git" || name == "vendor" || name == "node_modules" || name == "dist" {
		return filepath.SkipDir
	}
	return nil
}

func relevantHashFile(path string) bool {
	return strings.HasSuffix(path, ".go") || strings.HasSuffix(path, ".mod") || strings.HasSuffix(path, ".sum")
}

func hashWrite(h hash.Hash, s string) {
	_, _ = h.Write([]byte(s))
}

//nolint:ireturn // generic timing wrapper; T is whatever callee returns
func timePhase[T any](on bool, phase string, fn func() (T, error)) (T, error) {
	if !on {
		return fn()
	}
	start := time.Now() //nolint:forbidigo // reason: bootstrap layer owns clock reads
	out, err := fn()
	fmt.Fprintf(os.Stderr, phaseFmt, phase, time.Since(start).Round(time.Millisecond))
	return out, err
}

//nolint:ireturn // generic timing wrapper; A,B whatever callee returns
func timePhase2[A, B any](on bool, phase string, fn func() (A, B)) (A, B) {
	if !on {
		return fn()
	}
	start := time.Now() //nolint:forbidigo // reason: bootstrap layer owns clock reads
	a, b := fn()
	fmt.Fprintf(os.Stderr, phaseFmt, phase, time.Since(start).Round(time.Millisecond))
	return a, b
}

//nolint:ireturn // generic timing wrapper; T whatever callee returns
func timePhase3[T any](on bool, phase string, fn func() T) T {
	if !on {
		return fn()
	}
	start := time.Now() //nolint:forbidigo // reason: bootstrap layer owns clock reads
	out := fn()
	fmt.Fprintf(os.Stderr, phaseFmt, phase, time.Since(start).Round(time.Millisecond))
	return out
}

func idiomNotes(idiomErr error, issues []idiom.Issue) []string {
	var notes []string
	if idiomErr != nil {
		notes = append(notes, "idiom scan: "+idiomErr.Error())
	}
	if rendered := idiom.Format(issues); rendered != "" {
		notes = append(notes, rendered)
	}
	return notes
}
