package main

import (
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/config"
)

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

func TestConfigVerbPrintsTheGatesOwnConfig(t *testing.T) {
	t.Parallel()
	if len(config.GolangCI) == 0 {
		t.Fatal("the embedded golangci config is empty, so every consumer of the config verb measures the vendor default")
	}
	if !strings.Contains(string(config.GolangCI), "linters") {
		t.Error("the embedded config declares no linters section, so it cannot be the config this gate runs")
	}
}
