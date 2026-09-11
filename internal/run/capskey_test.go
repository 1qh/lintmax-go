package run

import "testing"

func TestTheCapabilityBaselineIsScopedToTheAnalyserThatProducedIt(t *testing.T) {
	t.Parallel()
	const dir = "/work/repo"
	if capsKey(dir, "1.0.0") == capsKey(dir, "1.1.0") {
		t.Fatal("an upgraded analyser must not inherit the previous baseline, or its wider report reads as a gain")
	}
	if capsKey(dir, "1.0.0") != capsKey(dir, "1.0.0") {
		t.Fatal("the same directory under the same analyser must compare against its own baseline")
	}
	if capsKey("/work/a", "1.0.0") == capsKey("/work/b", "1.0.0") {
		t.Fatal("two checkouts must keep separate baselines")
	}
}
