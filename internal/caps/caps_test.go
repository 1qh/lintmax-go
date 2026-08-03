package caps_test

import (
	"strings"
	"testing"

	"github.com/1qh/lintmax-go/internal/caps"
)

const (
	pkgA    = "example.com/a"
	pkgB    = "example.com/b"
	capNet  = "CAPABILITY_NETWORK"
	capFile = "CAPABILITY_FILES"
	sample  = `{"capabilityInfo":[
		{"packageName":"example.com/a","capability":"CAPABILITY_FILES","depPath":"x y z"},
		{"packageName":"example.com/a","capability":"CAPABILITY_FILES","depPath":"different path"},
		{"packageName":"example.com/b","capability":"CAPABILITY_NETWORK","depPath":"q"}
	]}`
)

func TestSetKeepsDistinctPairsAndDiscardsCallPaths(t *testing.T) {
	t.Parallel()
	got := caps.Set([]byte(sample))
	if len(got) != 2 {
		t.Fatalf("two distinct pairs expected from three entries, got %d: %v", len(got), got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, pkgA) || !strings.Contains(joined, capFile) {
		t.Fatalf("the file capability of package a must survive, got %v", got)
	}
	if strings.Contains(joined, "different path") {
		t.Fatalf("call paths must not enter the set, got %v", got)
	}
	if got[0] > got[1] {
		t.Fatalf("the set must be sorted so the comparison is stable, got %v", got)
	}
}

func TestSetRefusesToInventAnEmptySetFromGarbage(t *testing.T) {
	t.Parallel()
	if got := caps.Set([]byte("not json at all")); got != nil {
		t.Fatalf("unparsable output must yield nil, not an empty set, got %v", got)
	}
}

func TestGainedReportsAdditionsAndIgnoresRemovals(t *testing.T) {
	t.Parallel()
	before := []string{pkgA + "|" + capFile}
	now := []string{pkgA + "|" + capFile, pkgB + "|" + capNet}
	gained := caps.Gained(before, now)
	if len(gained) != 1 || !strings.Contains(gained[0], capNet) {
		t.Fatalf("the newly reachable network capability must be reported, got %v", gained)
	}
	if got := caps.Gained(now, before); len(got) != 0 {
		t.Fatalf("a removed capability must not fail anything, got %v", got)
	}
	if got := caps.Gained(now, now); len(got) != 0 {
		t.Fatalf("an unchanged set must be silent, got %v", got)
	}
}

func TestNoteNamesThePackageAndTheCapability(t *testing.T) {
	t.Parallel()
	got := caps.Note([]string{pkgB + "|" + capNet})
	if !strings.Contains(got, pkgB) || !strings.Contains(got, capNet) {
		t.Fatalf("the note must name package and capability, got %q", got)
	}
	if !strings.Contains(got, "GAINED") {
		t.Fatalf("the note must say the capability is new, got %q", got)
	}
}
