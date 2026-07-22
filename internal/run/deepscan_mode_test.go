//nolint:testpackage // reason: exercises the unexported deep-scan mode split
package run

import (
	"context"
	"strings"
	"testing"
)

func TestDeepScanSkipsTheCapabilityStageOnlyOnTheFastPath(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got := deepScan(ctx, false); got != nil {
		t.Fatalf("the fast path must run no dependency-scoped stage at all, got %v", got)
	}
	deep := strings.Join(deepScan(ctx, true), "\n")
	if !strings.Contains(deep, binCapslock) {
		t.Fatalf("the full path must still run the capability stage, got %q", deep)
	}
}
