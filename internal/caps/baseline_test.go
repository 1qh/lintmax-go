//nolint:testpackage // reason: exercises the pure unexported baseline comparison
package caps

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	baselinePairA = "example.com/a|CAPABILITY_FILES"
	baselinePairB = "example.com/b|CAPABILITY_NETWORK"
	baselinePairC = "example.com/c|CAPABILITY_SYSTEM_CALLS"
	baselineJSON  = "[\n  \"" + baselinePairA + "\",\n  \"" + baselinePairB + "\"\n]\n"
)

func readBaselineFixture(t *testing.T, path string) []byte {
	t.Helper()
	module, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		closeErr := module.Close()
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	})
	data, err := module.ReadFile(BaselineFile)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCompareBaseline(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		committed []string
		now       []string
		gained    []string
		lost      []string
	}{
		{
			name:      "equal",
			committed: []string{baselinePairA},
			now:       []string{baselinePairA},
			gained:    nil,
			lost:      nil,
		},
		{
			name:      "gained only",
			committed: []string{baselinePairA},
			now:       []string{baselinePairC, baselinePairA, baselinePairB},
			gained:    []string{baselinePairB, baselinePairC},
			lost:      nil,
		},
		{
			name:      "lost only",
			committed: []string{baselinePairC, baselinePairB, baselinePairA},
			now:       []string{baselinePairA},
			gained:    nil,
			lost:      []string{baselinePairB, baselinePairC},
		},
		{
			name:      "both",
			committed: []string{baselinePairA},
			now:       []string{baselinePairB},
			gained:    []string{baselinePairB},
			lost:      []string{baselinePairA},
		},
		{
			name:      "empty committed",
			committed: nil,
			now:       []string{baselinePairB, baselinePairA},
			gained:    []string{baselinePairA, baselinePairB},
			lost:      nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			gained, lost := compareBaseline(tc.committed, tc.now)
			if !slices.Equal(gained, tc.gained) || !slices.Equal(lost, tc.lost) {
				t.Fatalf("gained %v, lost %v; want gained %v, lost %v", gained, lost, tc.gained, tc.lost)
			}
		})
	}
}

func TestWriteBaselineHasCanonicalBytesAndFixedPoint(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), BaselineFile)
	entries := []string{baselinePairB, baselinePairA}
	err := WriteBaseline(path, entries)
	if err != nil {
		t.Fatal(err)
	}
	first := readBaselineFixture(t, path)
	if string(first) != baselineJSON {
		t.Fatalf("sorted, indented, newline-terminated baseline: got %q, want %q", first, baselineJSON)
	}
	if !slices.Equal(entries, []string{baselinePairB, baselinePairA}) {
		t.Fatal("writing must not reorder the caller's set")
	}
	err = WriteBaseline(path, []string{baselinePairA, baselinePairB})
	if err != nil {
		t.Fatal(err)
	}
	second := readBaselineFixture(t, path)
	if !bytes.Equal(first, second) {
		t.Fatalf("the second write must be byte-identical: first %q, second %q", first, second)
	}
}

func TestWriteBaselineEmptySetIsArray(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), BaselineFile)
	err := WriteBaseline(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw := readBaselineFixture(t, path)
	if string(raw) != "[]\n" {
		t.Fatalf("an empty capability set must be an array, got %q", raw)
	}
}

func TestCheckBaselineReportsBothDirectionsWithoutAcceptingThem(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), BaselineFile)
	err := WriteBaseline(path, []string{baselinePairA})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		note, checkErr := CheckBaseline(path, []string{baselinePairB})
		if checkErr != nil {
			t.Fatal(checkErr)
		}
		for _, want := range []string{Note([]string{baselinePairB}), "LOST", baselinePairA, fixBaseline} {
			if !strings.Contains(note, want) {
				t.Fatalf("both checks must retain %q in finding %q", want, note)
			}
		}
	}
	note, err := CheckBaseline(path, []string{baselinePairA})
	if err != nil || note != "" {
		t.Fatalf("the committed set must still be accepted: note %q, error %v", note, err)
	}
}
