package run

import "testing"

const (
	golangciPinEnv = "LINTMAX_PIN_GOLANGCI_LINT"
	golangciName   = "golangci-lint"
	pinnedVersionValue = "v2.12.2"
)

func TestAToolWithNoPinTracksLatest(t *testing.T) {
	t.Setenv(golangciPinEnv, "")
	if got := pinnedVersion(golangciName); got != latestVersion {
		t.Fatalf("an unpinned tool must track latest, or the gate silently stops gaining rules: got %q", got)
	}
}

func TestAPinnedToolInstallsExactlyThatVersion(t *testing.T) {
	t.Setenv(golangciPinEnv, pinnedVersionValue)
	if got := pinnedVersion(golangciName); got != pinnedVersionValue {
		t.Fatalf("a pin the consumer declared must be the version installed, or the pin buys nothing: got %q", got)
	}
}

func TestAPinNamesItsToolRatherThanApplyingToEveryTool(t *testing.T) {
	t.Setenv(golangciPinEnv, pinnedVersionValue)
	if got := pinnedVersion("nilaway"); got != latestVersion {
		t.Fatalf("a pin on one tool must leave its siblings on latest: got %q", got)
	}
}
