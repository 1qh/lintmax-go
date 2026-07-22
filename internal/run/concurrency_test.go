//nolint:testpackage // reason: exercises the unexported concurrency split
package run

import (
	"runtime"
	"testing"
)

// The linter shares host CPUs with the parallel test phase, which is correct only while that phase
// actually runs. The agent-side call skips tests, so halving the cores there throttles the gate's
// slowest remaining stage for a phase that is not running at all.
func TestLinterConcurrencyTakesTheWholeHostWhenTestsAreSkipped(t *testing.T) {
	t.Parallel()
	if got := linterConcurrency(true); got != runtime.NumCPU() {
		t.Fatalf("with no test phase the linter must use every core, want %d got %d", runtime.NumCPU(), got)
	}
}

// The negative control: while the test phase runs, the split must survive, or the two phases
// oversubscribe the host and both get slower.
func TestLinterConcurrencySharesTheHostWhileTestsRun(t *testing.T) {
	t.Parallel()
	got := linterConcurrency(false)
	if got >= runtime.NumCPU() && runtime.NumCPU() > minParallel*2 {
		t.Fatalf("while tests run the linter must not claim every core, got %d of %d", got, runtime.NumCPU())
	}
	if got < minParallel {
		t.Fatalf("the split must never fall below the floor %d, got %d", minParallel, got)
	}
}
