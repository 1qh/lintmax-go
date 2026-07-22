//nolint:testpackage // reason: exercises unexported analysisResult
package run

import (
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

const stageNilaway = "nilaway"

func TestAnalysisResultReportsAStageThatDidNotRun(t *testing.T) {
	t.Parallel()
	got := analysisResult(stageNilaway, nil, nil, false)
	if len(got.notes) != 1 {
		t.Fatalf("a failed analysis with no findings must raise a note, got %d", len(got.notes))
	}
	if !strings.Contains(got.notes[0], stageNilaway) {
		t.Fatalf("the note must name the stage that did not run, got %q", got.notes[0])
	}
	if !strings.Contains(got.notes[0], "DID NOT RUN") {
		t.Fatalf("the note must say the result is absent rather than clean, got %q", got.notes[0])
	}
}

func TestAnalysisResultExplainsAnEmptyFailure(t *testing.T) {
	t.Parallel()
	got := analysisResult("deadcode", nil, []byte("   \n"), false)
	if len(got.notes) != 1 || !strings.Contains(got.notes[0], "killed") {
		t.Fatalf("a silent kill must be explained, got %v", got.notes)
	}
}

func TestAnalysisResultStaysQuietWhenTheStageRan(t *testing.T) {
	t.Parallel()
	if got := analysisResult(stageNilaway, nil, []byte("anything"), true); len(got.notes) != 0 {
		t.Fatalf("a successful analysis must raise no note, got %v", got.notes)
	}
	found := []diag.Diagnostic{{File: "a.go", Linter: stageNilaway, Rule: stageNilaway, Line: 1}}
	got := analysisResult(stageNilaway, found, nil, false)
	if len(got.notes) != 0 {
		t.Fatalf("a failure that still reported findings needs no note, got %v", got.notes)
	}
	if len(got.diags) != 1 {
		t.Fatalf("the findings must survive, got %d", len(got.diags))
	}
}
