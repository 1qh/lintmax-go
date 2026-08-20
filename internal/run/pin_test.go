package run

import "testing"

func TestAToolWithNoPinTracksLatest(t *testing.T) {
	t.Setenv("LINTMAX_PIN_GOLANGCI_LINT", "")
	if got := pinnedVersion("golangci-lint"); got != latestVersion {
		t.Fatalf("an unpinned tool must track latest, or the gate silently stops gaining rules: got %q", got)
	}
}

func TestAPinnedToolInstallsExactlyThatVersion(t *testing.T) {
	t.Setenv("LINTMAX_PIN_GOLANGCI_LINT", "v2.12.2")
	if got := pinnedVersion("golangci-lint"); got != "v2.12.2" {
		t.Fatalf("a pin the consumer declared must be the version installed, or the pin buys nothing: got %q", got)
	}
}

func TestAPinNamesItsToolRatherThanApplyingToEveryTool(t *testing.T) {
	t.Setenv("LINTMAX_PIN_GOLANGCI_LINT", "v2.12.2")
	if got := pinnedVersion("nilaway"); got != latestVersion {
		t.Fatalf("a pin on one tool must leave its siblings on latest: got %q", got)
	}
}
