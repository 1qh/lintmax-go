package diag_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

const (
	findingPackageDir = "server"
	findingSourceFile = "share.go"
)

func TestAFindingNamesTheFileThatHoldsIt(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	mkErr := os.MkdirAll(filepath.Join(root, findingPackageDir), 0o750)
	if mkErr != nil {
		t.Fatalf("make the package directory: %v", mkErr)
	}
	nested := filepath.Join(root, findingPackageDir, findingSourceFile)
	wErr := os.WriteFile(nested, []byte("package server\n"), 0o600)
	if wErr != nil {
		t.Fatalf("write the nested file: %v", wErr)
	}
	raw, merr := json.Marshal(map[string]any{"Issues": []map[string]any{
		{
			"FromLinter": "revive", "Text": "identical branches (lines 72 and 84)",
			"Pos": map[string]any{"Filename": nested, "Line": 71},
		},
	}})
	if merr != nil {
		t.Fatalf("build the golangci payload: %v", merr)
	}
	diags := diag.ParseGolangci(raw)
	if len(diags) != 1 {
		t.Fatalf("one issue must parse to one diagnostic, got %d", len(diags))
	}
	got := diag.Format(diags, root)
	if !strings.Contains(got, filepath.Join(findingPackageDir, findingSourceFile)) {
		t.Fatalf("the finding must name the file that holds it, got %q", got)
	}
}
