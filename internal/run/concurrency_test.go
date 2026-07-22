//nolint:testpackage // reason: exercises the unexported concurrency split
package run

import (
	"runtime"
	"testing"
)

func TestLinterConcurrencyTakesTheWholeHostWhenTestsAreSkipped(t *testing.T) {
	t.Parallel()
	if got := linterConcurrency(true); got != runtime.NumCPU() {
		t.Fatalf("with no test phase the linter must use every core, want %d got %d", runtime.NumCPU(), got)
	}
}

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
