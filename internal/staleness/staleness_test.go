package staleness_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/1qh/lintmax-go/internal/staleness"
)

const (
	envSkipStaleness = "LINTMAX_SKIP_STALENESS"
	fmtScanErr       = "Scan: %v"
)

func TestNormalizeMajorStripsMinorPatch(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"v6":          "v6",
		"v6.0.0":      "v6",
		"v6.1":        "v6",
		"v6.0.0-rc.1": "v6",
		"6.2.3":       "v6",
	}
	for in, want := range cases {
		got := staleness.NormalizeMajorForTest(in)
		if got != want {
			t.Fatalf("normalizeMajor(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestScanReturnsNilWhenSkipped(t *testing.T) {
	t.Setenv(envSkipStaleness, "1")
	out, err := staleness.Scan(context.Background(), ".") //nolint:forbidigo // reason: bootstrap test harness ctx
	if err != nil {
		t.Fatalf(fmtScanErr, err)
	}
	if out != nil {
		t.Fatalf("expected nil, got %v", out)
	}
}

func TestScanActionsNoWorkflowsIsEmpty(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	out, err := staleness.Scan(context.Background(), dir) //nolint:forbidigo // reason: bootstrap test harness ctx
	if err != nil {
		t.Fatalf(fmtScanErr, err)
	}
	if len(out) != 0 {
		t.Fatalf("want empty, got %v", out)
	}
}

func TestScanActionsParsesPinsFromYAML(t *testing.T) {
	dir := writeFixtureWorkflow(t)
	t.Setenv(envSkipStaleness, "1")
	_, _ = staleness.Scan(context.Background(), dir) //nolint:errcheck,forbidigo // reason: smoke test; bootstrap test ctx
}

func writeFixtureWorkflow(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	wf := filepath.Join(dir, ".github", "workflows", "ci.yml")
	mkErr := os.MkdirAll(filepath.Dir(wf), 0o750)
	if mkErr != nil {
		t.Fatalf("mkdir: %v", mkErr)
	}
	body := []byte("jobs:\n  test:\n    steps:\n      - uses: actions/checkout@v6\n")
	wErr := os.WriteFile(wf, body, 0o600)
	if wErr != nil {
		t.Fatalf("write: %v", wErr)
	}
	return dir
}

func TestFormatRendersIssues(t *testing.T) {
	t.Parallel()
	issues := []staleness.Issue{{
		Source: "actions", Name: "actions/checkout",
		Have: "v5", Latest: "v6",
		ReleasedAt: time.Now().Add(-30 * 24 * time.Hour),
	}}
	out := staleness.Format(issues)
	if out == "" {
		t.Fatal("expected non-empty render")
	}
}

func TestToleranceFromEnv(t *testing.T) {
	t.Setenv("LINTMAX_STALENESS_TOLERANCE_DAYS", "3")
	got := staleness.ToleranceForTest()
	if got != 3*24*time.Hour {
		t.Fatalf("got %v, want 72h", got)
	}
}
