package diag_test

import (
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

const (
	probeLinter = "nilaway"
	fileA       = "/x/a.go"
	fileB       = "/x/b.go"
	fileC       = "/x/c.go"
	boomMsg     = `"message":"boom"`
	oneDoc      = `{"pkg/a":{"nilaway":[{"posn":"/x/a.go:9:10",` + boomMsg + `}]}}`
)

func TestParseAnalysisReadsEveryDocumentInAStream(t *testing.T) {
	t.Parallel()
	stream := oneDoc + "\n" +
		`{"pkg/b":{"nilaway":[{"posn":"/x/b.go:4:2",` + boomMsg + `}]}}` + "\n" +
		`{"pkg/c":{"nilaway":[{"posn":"/x/c.go:7:1",` + boomMsg + `},` +
		`{"posn":"/x/c.go:8:1",` + boomMsg + `}]}}` + "\n"
	got := diag.ParseAnalysis([]byte(stream), probeLinter)
	if len(got) != 4 {
		t.Fatalf("every finding across every document must survive, want 4 got %d", len(got))
	}
	files := map[string]bool{}
	for _, d := range got {
		files[d.File] = true
	}
	for _, want := range []string{fileA, fileB, fileC} {
		if !files[want] {
			t.Fatalf("a document later in the stream was dropped: %s missing from %v", want, files)
		}
	}
}

func TestParseAnalysisStillReadsASingleDocument(t *testing.T) {
	t.Parallel()
	got := diag.ParseAnalysis([]byte(oneDoc), probeLinter)
	if len(got) != 1 {
		t.Fatalf("a single document must still parse, want 1 got %d", len(got))
	}
	if got[0].File != fileA || got[0].Line != 9 {
		t.Fatalf("position must survive, got %+v", got[0])
	}
	if got[0].Rule != probeLinter {
		t.Fatalf("the analyzer name must become the rule, got %q", got[0].Rule)
	}
}

func TestParseAnalysisHandlesAStreamOfEmptyDocuments(t *testing.T) {
	t.Parallel()
	if got := diag.ParseAnalysis([]byte("{}\n{}\n{}\n"), probeLinter); len(got) != 0 {
		t.Fatalf("clean packages must yield no findings, got %v", got)
	}
	if got := diag.ParseAnalysis(nil, probeLinter); len(got) != 0 {
		t.Fatalf("no output must yield no findings, got %v", got)
	}
}
