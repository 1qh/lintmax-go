package diag_test

import (
	"testing"

	"github.com/1qh/lintmax-go/internal/diag"
)

const linterNilaway = "nilaway"

const foreignFinding = `{"m":{"nilaway":[{"posn":"/repo/metrics.go:127:3","message":"Potential nil panic detected. ` +
	`Observed nil flow from source to dereference point: \n\t- http/transport.go:716:11: result 0 of ` + "`roundTrip()`" +
	` lacking guarding\n\t- storage@v1.64.0/metrics.go:605:16: result 0 of ` + "`RoundTrip()`" + ` accessed field ` +
	"`Body`" + `\n"}]}}`

const localFinding = `{"m":{"nilaway":[{"posn":"/repo/store.go:40:2","message":"Potential nil panic detected. ` +
	`Observed nil flow from source to dereference point: \n\t- storage@v1.64.0/client.go:10:3: result 0 of ` +
	"`New()`" + ` lacking guarding\n\t- /repo/store.go:40:2: result 0 of ` + "`New()`" + ` accessed field ` +
	"`Bucket`" + `\n"}]}}`

func TestANilFlowDereferencedOnlyInsideADependencyIsNotThisRepositorysFinding(t *testing.T) {
	t.Parallel()
	if got := diag.ParseAnalysis([]byte(foreignFinding), linterNilaway); len(got) != 0 {
		t.Fatalf("a dependency-only dereference must not reach the gate, got %d", len(got))
	}
}

func TestAFlowThroughADependencyThatDereferencesHereStillFails(t *testing.T) {
	t.Parallel()
	got := diag.ParseAnalysis([]byte(localFinding), linterNilaway)
	if len(got) != 1 {
		t.Fatalf("a local dereference must survive however far its flow travelled, got %d", len(got))
	}
	if got[0].Line != 40 {
		t.Fatalf("the finding must keep its own position, got line %d", got[0].Line)
	}
}

func TestADiagnosticWithNoDereferenceIsUntouched(t *testing.T) {
	t.Parallel()
	plain := `{"m":{"nilaway":[{"posn":"/repo/a.go:3:1","message":"something else entirely"}]}}`
	if got := diag.ParseAnalysis([]byte(plain), linterNilaway); len(got) != 1 {
		t.Fatalf("a diagnostic that names no dereference must be reported, got %d", len(got))
	}
}
