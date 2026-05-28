package transform_test

import (
	"testing"

	"github.com/1qh/lintmax-go/internal/transform"
)

const fmtGotWant = "got %q want %q"

func TestStripCommentsRemovesLineAndBlock(t *testing.T) {
	t.Parallel()
	src := []byte("package x\n\n// a doc comment\nfunc A() {} /* trailing */\n")
	got := string(transform.StripComments(src))
	if want := "package x\n\nfunc A() {} \n"; got != want {
		t.Fatalf(fmtGotWant, got, want)
	}
}

func TestStripCommentsKeepsDirectives(t *testing.T) {
	t.Parallel()
	src := "package x\n\n//go:embed f\nvar f []byte //nolint:gochecknoglobals // ok\n"
	if got := string(transform.StripComments([]byte(src))); got != src {
		t.Fatalf("directives must survive: got %q", got)
	}
}

func TestStripCommentsSkipsCgo(t *testing.T) {
	t.Parallel()
	src := "package x\n\n// #include <stdio.h>\nimport \"C\"\n"
	if got := string(transform.StripComments([]byte(src))); got != src {
		t.Fatal("cgo files must be left untouched")
	}
}

func TestCompactCollapsesTopLevelBlanks(t *testing.T) {
	t.Parallel()
	src := []byte("package x\n\n\n\nvar a = 1\n")
	if want, got := "package x\n\nvar a = 1\n", string(transform.Compact(src)); got != want {
		t.Fatalf(fmtGotWant, got, want)
	}
}

func TestCompactRemovesIntraBodyBlanks(t *testing.T) {
	t.Parallel()
	src := []byte("package x\n\nfunc A() {\n\tx := 1\n\n\ty := 2\n\t_ = x\n\t_ = y\n}\n")
	want := "package x\n\nfunc A() {\n\tx := 1\n\ty := 2\n\t_ = x\n\t_ = y\n}\n"
	if got := string(transform.Compact(src)); got != want {
		t.Fatalf("intra-body blank not removed: got %q want %q", got, want)
	}
}

func TestCompactKeepsTopLevelBlankBetweenDecls(t *testing.T) {
	t.Parallel()
	src := "package x\n\nfunc A() {}\n\nfunc B() {}\n"
	if got := string(transform.Compact([]byte(src))); got != src {
		t.Fatalf("top-level blank (gofmt-mandated) must stay: got %q", got)
	}
}
