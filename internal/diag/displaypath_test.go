package diag_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

// TWO FILES SHARING A BASENAME MUST NOT SHARE A DISPLAYED NAME. golangci reports a filename relative
// to each PACKAGE, so a finding in `server/share.go` arrives as bare `share.go` and is displayed as
// the root `share.go` — a reader then hunts the defect in a file that does not contain it. The gate
// asks golangci for absolute paths so the display can carry enough of the path to tell them apart.
func TestAFindingNamesTheFileThatHoldsIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// The file must EXIST: the display resolves symlinks on both sides, and a path that cannot be
	// resolved falls back to the basename — which would pass this test for the wrong reason.
	if mkErr := os.MkdirAll(filepath.Join(root, "server"), 0o750); mkErr != nil {
		t.Fatalf("make the package directory: %v", mkErr)
	}
	nested := filepath.Join(root, "server", "share.go")
	if wErr := os.WriteFile(nested, []byte("package server\n"), 0o600); wErr != nil {
		t.Fatalf("write the nested file: %v", wErr)
	}
	raw, merr := json.Marshal(map[string]any{"Issues": []map[string]any{
		{"FromLinter": "revive", "Text": "identical branches (lines 72 and 84)",
			"Pos": map[string]any{"Filename": nested, "Line": 71}},
	}})
	if merr != nil {
		t.Fatalf("build the golangci payload: %v", merr)
	}
	diags := diag.ParseGolangci(raw)
	if len(diags) != 1 {
		t.Fatalf("one issue must parse to one diagnostic, got %d", len(diags))
	}
	got := diag.Format(diags, root)
	if !strings.Contains(got, filepath.Join("server", "share.go")) {
		t.Fatalf("the finding must name the file that holds it, got %q", got)
	}
}
