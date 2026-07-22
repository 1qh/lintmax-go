package stage_test

import (
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/stage"
)

const (
	probe    = "nilaway"
	tailText = "some tail output"
)

func TestAbsentSeparatesADeadStageFromACleanOne(t *testing.T) {
	t.Parallel()
	if !stage.Absent(false, 0) {
		t.Fatal("a stage that failed with no findings has not run, and must read as absent")
	}
	if stage.Absent(true, 0) {
		t.Fatal("a stage that succeeded with no findings is genuinely clean")
	}
	if stage.Absent(false, 3) {
		t.Fatal("a stage that failed WITH findings is an ordinary gate failure, not an absent one")
	}
	if stage.Absent(true, 3) {
		t.Fatal("a stage that succeeded and reported findings is not absent")
	}
}

func TestNoteNamesTheStageAndSaysTheResultIsAbsent(t *testing.T) {
	t.Parallel()
	got := stage.Note(probe, tailText)
	if !strings.Contains(got, probe) {
		t.Fatalf("the note must name the stage, got %q", got)
	}
	if !strings.Contains(got, "DID NOT RUN") {
		t.Fatalf("the note must say the result is absent rather than clean, got %q", got)
	}
	if !strings.Contains(got, tailText) {
		t.Fatalf("the note must carry whatever the stage emitted, got %q", got)
	}
}

func TestNoteExplainsASilentKill(t *testing.T) {
	t.Parallel()
	for _, empty := range []string{"", "   \n\t "} {
		if got := stage.Note("deadcode", empty); !strings.Contains(got, "killed") {
			t.Fatalf("a silent kill must be explained, got %q", got)
		}
	}
}
