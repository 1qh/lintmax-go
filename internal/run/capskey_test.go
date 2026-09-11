package run //nolint:testpackage // reason: exercises unexported capsKey

import "testing"

const capsTestAnalyser = "1.0.0"

func TestTheCapabilityBaselineIsScopedToTheAnalyserThatProducedIt(t *testing.T) {
	t.Parallel()
	const dir = "/work/repo"
	first := capsKey(dir, capsTestAnalyser)
	again := capsKey(dir, capsTestAnalyser)
	if first != again {
		t.Fatal("the same directory under the same analyser must compare against its own baseline")
	}
	if first == capsKey(dir, "1.1.0") {
		t.Fatal("an upgraded analyser must not inherit the previous baseline, or its wider report reads as a gain")
	}
	if first == capsKey("/work/other", capsTestAnalyser) {
		t.Fatal("two checkouts must keep separate baselines")
	}
}
