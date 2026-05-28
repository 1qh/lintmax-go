package floatdiv_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/1qh/lintmax-go/internal/floatdiv"
)

func writeMod(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module fdsmoke\n\ngo 1.23\n",
		"a.go":   src,
	}
	for name, body := range files {
		err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestFlagsUnguarded(t *testing.T) {
	t.Parallel()
	src := "package p\n" +
		"func Bad(rows []int) float64 { var s float64; for range rows { s++ }; return s / float64(len(rows)) }\n"
	is, err := floatdiv.Scan(t.Context(), writeMod(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if len(is) != 1 {
		t.Fatalf("want 1 unguarded finding, got %d: %+v", len(is), is)
	}
}

func TestSkipsGuarded(t *testing.T) {
	t.Parallel()
	src := "package p\n" +
		"func Good(rows []int) float64 { if len(rows) == 0 { return 0 }; var s float64; return s / float64(len(rows)) }\n"
	is, err := floatdiv.Scan(t.Context(), writeMod(t, src))
	if err != nil {
		t.Fatal(err)
	}
	if len(is) != 0 {
		t.Fatalf("guarded div must not flag, got %+v", is)
	}
}
