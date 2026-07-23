package diag_test

import (
	"path/filepath"
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

func TestADependencysOwnFileIsNotThisRepositorysFinding(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	dep := filepath.Join(t.TempDir(), "pkg", "mod", "cloud.example", "storage@v1", "metrics.go")
	if !diag.Outside(root, dep) {
		t.Fatal("a file outside the module root is a dependency's, not this repository's")
	}
	mine := filepath.Join(root, "internal", "run", "run.go")
	if diag.Outside(root, mine) {
		t.Fatal("a file under the module root is ours and must still be reported")
	}
	if diag.Outside(root, "relative/path.go") {
		t.Fatal("a relative path is already repository-relative and must be kept")
	}
	if diag.Outside(root, "") {
		t.Fatal("an empty path carries no evidence of being foreign")
	}
}
