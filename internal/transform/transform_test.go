package transform_test

import (
	"os"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/transform"
)

const (
	fmtGotWant  = "got %q want %q"
	fixtureFile = "fixture.go"
)

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

func TestCompactKeepsTheFormatterSeparatorAfterAnEmbeddedField(t *testing.T) {
	t.Parallel()
	src := "package p\n\nfunc f() {\n\ttype row struct {\n\t\tbase\n\n\t\tName string\n\t}\n\n\t_ = row{}\n}\n"
	want := "package p\n\nfunc f() {\n\ttype row struct {\n\t\tbase\n\n\t\tName string\n\t}\n\t_ = row{}\n}\n"
	if got := string(transform.Compact([]byte(src))); got != want {
		t.Fatalf("Compact removed the separator the formatter re-adds:\n%q\nwant\n%q", got, want)
	}
	if again := string(transform.Compact([]byte(want))); again != want {
		t.Fatalf("Compact is not a fixed point:\n%q", again)
	}
}

func TestCompactKeepsBlankLinesInsideRawStrings(t *testing.T) {
	t.Parallel()
	const src = "package x\n\nfunc f() string {\n\treturn `a\n\nb\n\n\n\nc`\n}\n\nvar v = `d\n\n\n\ne`\n"
	if got := string(transform.Compact([]byte(src))); got != src {
		t.Fatalf("a raw string literal is a value, so its blank lines must survive:\n%q\ngot\n%q", src, got)
	}
}

func TestCompactPreservesRawStringBytes(t *testing.T) {
	t.Parallel()
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(fmtNewlineName(newline), func(t *testing.T) {
			t.Parallel()
			literal := strings.ReplaceAll("`\n\nfirst\n\n\n\nlast\n\n`", "\n", newline)
			src := "package x\n\n\n\nvar fixture = " + literal +
				"\n\nfunc f() string {\n\n\treturn " + literal + "\n\n}\n"
			want := "package x\n\nvar fixture = " + literal +
				"\n\nfunc f() string {\n\treturn " + literal + "\n}\n"
			if got := string(transform.Compact([]byte(src))); got != want {
				t.Fatalf(fmtGotWant, got, want)
			}
		})
	}
}

func TestCompactPreservesBlockCommentBytes(t *testing.T) {
	t.Parallel()
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(fmtNewlineName(newline), func(t *testing.T) {
			t.Parallel()
			comment := strings.ReplaceAll("/* #fixture\n\nfirst\n\n\n\nlast\n\n*/", "\n", newline)
			src := "package x\n\n\n\n" + comment + "\nfunc f() {\n\n" + comment + "\n\n}\n"
			want := "package x\n\n" + comment + "\nfunc f() {\n" + comment + "\n}\n"
			if got := string(transform.Compact([]byte(src))); got != want {
				t.Fatalf(fmtGotWant, got, want)
			}
			if got := string(transform.Compact(transform.StripComments([]byte(src)))); got != want {
				t.Fatalf("retained directive: "+fmtGotWant, got, want)
			}
		})
	}
}

func TestStripCommentsUsesSourceByteBoundaries(t *testing.T) {
	t.Parallel()
	literal := "`\r\n\r\n// fixture\r\n/* fixture */\r\n\r\n`"
	src := "package x\nvar fixture = " + literal + "\n/* remove\r\n\r\nthis */\n// remove\r\nvar n = 1\n"
	want := "package x\nvar fixture = " + literal + "\nvar n = 1\n"
	if got := string(transform.StripComments([]byte(src))); got != want {
		t.Fatalf(fmtGotWant, got, want)
	}
}

func TestApplyPreservesRawStringFixture(t *testing.T) {
	t.Parallel()
	literal := "`\r\n\r\nfirst\r\n\r\n\r\n\r\nlast\r\n\r\n`"
	src := "package x\n\n\n\nfunc fixture() string {\n\n\treturn " + literal + "\n\n}\n"
	want := "package x\n\nfunc fixture() string {\n\treturn " + literal + "\n}\n"
	root := t.TempDir()
	module, err := os.OpenRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeErr := module.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	})
	writeErr := module.WriteFile(fixtureFile, []byte(src), 0o600)
	if writeErr != nil {
		t.Fatal(writeErr)
	}
	_, applyErr := transform.Apply(root)
	if applyErr != nil {
		t.Fatal(applyErr)
	}
	got, err := module.ReadFile(fixtureFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf(fmtGotWant, got, want)
	}
}

func TestCompactPreservesUnterminatedTokens(t *testing.T) {
	t.Parallel()
	for _, prefix := range []string{"package x\nvar fixture = `", "package x\n/*"} {
		t.Run(prefix, func(t *testing.T) {
			t.Parallel()
			src := prefix + "\r\n\r\n\r\n\r\n\r\n\r\n\r\n\r\n"
			if got := string(transform.Compact([]byte(src))); got != src {
				t.Fatalf(fmtGotWant, got, src)
			}
		})
	}
}

func TestStripCommentsRemovesCommentsAtEOF(t *testing.T) {
	t.Parallel()
	for _, comment := range []string{"// remove\r", "/* remove\r\n", "/* remove\r\n*/"} {
		t.Run(comment, func(t *testing.T) {
			t.Parallel()
			const want = "package x\n"
			if got := string(transform.StripComments([]byte(want + comment))); got != want {
				t.Fatalf(fmtGotWant, got, want)
			}
		})
	}
}

func fmtNewlineName(newline string) string {
	if newline == "\r\n" {
		return "CRLF"
	}
	return "LF"
}
