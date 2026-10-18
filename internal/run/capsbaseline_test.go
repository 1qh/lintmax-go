//nolint:testpackage // reason: exercises the production capability scan and baseline handling
package run

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/caps"
)

const (
	capsBaselinePair   = "example.com/app|CAPABILITY_FILES"
	capsBaselineReport = `{"capabilityInfo":[{"packageName":"example.com/app","capability":"CAPABILITY_FILES"}]}`
	capsEmptyScript    = "printf '%s\\n' '{\"capabilityInfo\":[]}'\n"
	capsFixCommand     = "lintmax-go fix"
)

func installCapsFixture(t *testing.T, script string) {
	t.Helper()
	toolDir := t.TempDir()
	t.Setenv(goBinEnv, toolDir)
	err := os.WriteFile(filepath.Join(toolDir, binCapslock), []byte("#!/bin/sh\n"+script), privateDirMode)
	if err != nil {
		t.Fatal(err)
	}
}

func TestCapabilityCheckMissingBaselineRequiresFix(t *testing.T) {
	t.Chdir(t.TempDir())
	installCapsFixture(t, "printf '%s\\n' '"+capsBaselineReport+"'\n")
	for range 2 {
		note := capabilityScan(t.Context())
		if !strings.Contains(note, caps.BaselineFile) || !strings.Contains(note, capsFixCommand) {
			t.Fatalf("a missing baseline must name its file and creation command: %q", note)
		}
	}
	_, err := os.Stat(caps.BaselineFile)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("check must not create its missing baseline: %v", err)
	}
}

func TestCapabilityCheckReportsLostEntriesFromEmptyScan(t *testing.T) {
	t.Chdir(t.TempDir())
	installCapsFixture(t, capsEmptyScript)
	err := caps.WriteBaseline(caps.BaselineFile, []string{capsBaselinePair})
	if err != nil {
		t.Fatal(err)
	}
	note := capabilityScan(t.Context())
	for _, want := range []string{"LOST", capsBaselinePair, capsFixCommand} {
		if !strings.Contains(note, want) {
			t.Fatalf("empty scans must expose stale entries and the repair command %q: %q", want, note)
		}
	}
}

func TestCapabilityFixPanicPreservesBaseline(t *testing.T) {
	t.Chdir(t.TempDir())
	installCapsFixture(t, "printf '%s\\n' '"+ssaPanicMarker+"' >&2\nexit 1\n")
	before := []byte("[\n  \"" + capsBaselinePair + "\"\n]\n")
	err := os.WriteFile(caps.BaselineFile, before, configMode)
	if err != nil {
		t.Fatal(err)
	}
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: "", skipCheck: false, greenKey: ""}
	err = g.refreshCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(caps.BaselineFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("an unreadable scan must preserve baseline bytes: got %q, want %q", after, before)
	}
}

func TestCapabilityFixWritesEmptyScan(t *testing.T) {
	t.Chdir(t.TempDir())
	installCapsFixture(t, capsEmptyScript)
	g := &gateCtx{ctx: t.Context(), timing: false, fix: true, cfg: "", skipCheck: false, greenKey: ""}
	err := g.refreshCapabilities()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(caps.BaselineFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != "[]\n" {
		t.Fatalf("fix must persist an empty capability set as an array: %q", raw)
	}
}
