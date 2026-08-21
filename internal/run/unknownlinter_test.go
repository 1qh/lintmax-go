package run

import (
	"strings"
	"testing"
)

// A disable entry the resolved golangci-lint does not know refuses the WHOLE run with
// `unknown linters`, so a config written for a newer release breaks every consumer holding a pin.
func TestAnUnknownDisableEntryIsDropped(t *testing.T) {
	cfg := "linters:\n  disable:\n    - wsl # keep\n    - notalinteratall # drop\n"
	known := map[string]bool{"wsl": true}
	out := []string{}
	for _, line := range strings.Split(cfg, "\n") {
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
	if !strings.Contains(kept, "- wsl") {
		t.Fatal("a known disable entry was dropped, which is a strictness loss wearing a compatibility fix")
	}
}

func TestADisableEntryIsToldFromAnOrdinaryListItem(t *testing.T) {
	if disabledLinterName("    - 'some/pattern'") != "" {
		t.Fatal("a quoted list item reads as a linter name, so unrelated config would be dropped")
	}
	if disabledLinterName("    - wsl_v5 # successor") != "wsl_v5" {
		t.Fatal("a commented disable entry must still resolve to its name")
	}
}

// The filter must reach ONLY the linters-disable block: every other `- name` list in the config
// names a CHECK rather than a linter, so a tree-wide filter re-enables them — measured at 619
// findings on a tree the same gate had just called clean.
func TestOnlyTheLintersDisableBlockIsFiltered(t *testing.T) {
	cfg := strings.Join([]string{
		"linters:",
		"  disable:",
		"    - wsl",
		"    - notalinter",
		"  settings:",
		"    gocritic:",
		"      disabled-checks:",
		"        - hugeParam",
		"        - rangeValCopy",
	}, "\n")
	kept := dropUnknownLintersWith(cfg, map[string]bool{"wsl": true})
	if strings.Contains(kept, "notalinter") {
		t.Fatal("an unknown linter survived the disable block, so a pinned consumer's run is refused")
	}
	for _, check := range []string{"hugeParam", "rangeValCopy"} {
		if !strings.Contains(kept, check) {
			t.Fatalf("%s was dropped, which re-enables a check the gate deliberately disables", check)
		}
	}
}
