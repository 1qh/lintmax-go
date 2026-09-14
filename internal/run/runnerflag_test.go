//nolint:testpackage // reason: exercises the unexported runner flag choice
package run

import (
	"slices"
	"testing"
)

func TestAnIsolatedCIRunNeverRefusesOnAnotherRunnersLock(t *testing.T) {
	t.Parallel()
	if got := golangciRunnerFlag(true); got != "--allow-parallel-runners" {
		t.Fatalf("a CI run owns its cache, so it must run beside another instance, got %q", got)
	}
}

func TestALocalRunWaitsForTheSharedCacheLockInsteadOfFailing(t *testing.T) {
	t.Parallel()
	if got := golangciRunnerFlag(false); got != "--allow-serial-runners" {
		t.Fatalf("a local run shares one cache, so it must queue on the lock rather than refuse, got %q", got)
	}
}

func TestTheGolangciInvocationCarriesTheRunnerFlag(t *testing.T) {
	t.Parallel()
	for _, ci := range []bool{true, false} {
		if args := golangciArgs("cfg.yml", false, ci); !slices.Contains(args, golangciRunnerFlag(ci)) {
			t.Fatalf("the golangci invocation must carry %q, got %v", golangciRunnerFlag(ci), args)
		}
	}
}
