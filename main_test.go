package main

import "testing"

func TestRealMainNoArgsIsUsage(t *testing.T) {
	t.Parallel()

	got := realMain(nil)
	if got != exitUsage {
		t.Fatalf("no args: got exit %d, want %d", got, exitUsage)
	}
}

func TestRealMainUnknownIsUsage(t *testing.T) {
	t.Parallel()

	got := realMain([]string{"bogus"})
	if got != exitUsage {
		t.Fatalf("unknown cmd: got exit %d, want %d", got, exitUsage)
	}
}

func TestReportNilIsOK(t *testing.T) {
	t.Parallel()

	if got := report(nil); got != exitOK {
		t.Fatalf("nil err: got exit %d, want %d", got, exitOK)
	}
}
