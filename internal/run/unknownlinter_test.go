package run //nolint:testpackage // reason: exercises unexported disabledLinterName and dropUnknownLintersWith

import (
	"strings"
	"testing"
)

const knownDisabledLinter = "wsl"

func TestAnUnknownDisableEntryIsDropped(t *testing.T) {
	t.Parallel()
	cfg := "linters:\n  disable:\n    - " + knownDisabledLinter + " # keep\n    - notalinteratall # drop\n"
	known := map[string]bool{knownDisabledLinter: true}
	out := []string{}
	for line := range strings.SplitSeq(cfg, "\n") {
		name := disabledLinterName(line)
		if name != "" && !known[name] {
			continue
		}
		out = append(out, line)
	}
	kept := strings.Join(out, "\n")
	if strings.Contains(kept, "notalinteratall") {
		t.Fatal("an unknown disable entry survived, so a pinned consumer's run is refused outright")
	}
	if !strings.Contains(kept, "- "+knownDisabledLinter) {
		t.Fatal("a known disable entry was dropped, which is a strictness loss wearing a compatibility fix")
	}
}

func TestADisableEntryIsToldFromAnOrdinaryListItem(t *testing.T) {
	t.Parallel()
	if disabledLinterName("    - 'some/pattern'") != "" {
		t.Fatal("a quoted list item reads as a linter name, so unrelated config would be dropped")
	}
	if disabledLinterName("    - wsl_v5 # successor") != "wsl_v5" {
		t.Fatal("a commented disable entry must still resolve to its name")
	}
}

func TestOnlyTheLintersDisableBlockIsFiltered(t *testing.T) {
	t.Parallel()
	cfg := strings.Join([]string{
		"linters:",
		"  disable:",
		"    - " + knownDisabledLinter,
		"    - notalinter",
		"  settings:",
		"    gocritic:",
		"      disabled-checks:",
		"        - hugeParam",
		"        - rangeValCopy",
	}, "\n")
	kept := dropUnknownLintersWith(cfg, map[string]bool{knownDisabledLinter: true})
	if strings.Contains(kept, "notalinter") {
		t.Fatal("an unknown linter survived the disable block, so a pinned consumer's run is refused")
	}
	for _, check := range []string{"hugeParam", "rangeValCopy"} {
		if !strings.Contains(kept, check) {
			t.Fatalf("%s was dropped, which re-enables a check the gate deliberately disables", check)
		}
	}
}
