package caps

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	pairSep      = "|"
	BaselineFile = ".lintmax-caps.json"
	fixBaseline  = "run lintmax-go fix"
	baselineMode = 0o600
)

type reportEntry struct {
	PackageName string `json:"packageName"`
	Capability  string `json:"capability"`
}

type report struct {
	CapabilityInfo []reportEntry `json:"capabilityInfo"`
}

func Set(raw []byte) []string {
	var rep report
	if json.Unmarshal(raw, &rep) != nil {
		return nil
	}
	seen := map[string]struct{}{}
	for _, e := range rep.CapabilityInfo {
		if e.PackageName == "" || e.Capability == "" {
			continue
		}
		seen[e.PackageName+pairSep+e.Capability] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func Gained(before, now []string) []string {
	had := make(map[string]struct{}, len(before))
	for _, b := range before {
		had[b] = struct{}{}
	}
	var gained []string
	for _, n := range now {
		if _, ok := had[n]; !ok {
			gained = append(gained, n)
		}
	}
	return gained
}

func Note(gained []string) string {
	var b strings.Builder
	b.WriteString("capability GAINED since the last run — a package now reaches something it did not:\n")
	for _, g := range gained {
		pkg, capability, _ := strings.Cut(g, pairSep)
		b.WriteString("  " + pkg + " -> " + capability + "\n")
	}
	b.WriteString("  (" + fixBaseline + " and commit " + BaselineFile +
		" to accept the gained set as the baseline once the change is understood)")
	return b.String()
}

func compareBaseline(committed, now []string) ([]string, []string) {
	gained := Gained(committed, now)
	lost := Gained(now, committed)
	slices.Sort(gained)
	slices.Sort(lost)
	return gained, lost
}

func CheckBaseline(path string, now []string) (string, error) {
	raw, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return BaselineFile + " missing: " + fixBaseline + " to create the capability baseline", nil
	}
	if err != nil {
		return "", fmt.Errorf("read capability baseline: %w", err)
	}
	var committed []string
	err = json.Unmarshal(raw, &committed)
	if err != nil {
		return "", fmt.Errorf("decode capability baseline: %w", err)
	}
	gained, lost := compareBaseline(committed, now)
	var notes []string
	if len(gained) > 0 {
		notes = append(notes, Note(gained))
	}
	if len(lost) > 0 {
		notes = append(notes, "capability LOST (stale baseline):\n  "+strings.Join(lost, "\n  ")+"\n  "+fixBaseline)
	}
	return strings.Join(notes, "\n"), nil
}

func WriteBaseline(path string, entries []string) error {
	sorted := make([]string, len(entries))
	copy(sorted, entries)
	slices.Sort(sorted)
	raw, err := json.MarshalIndent(sorted, "", "  ")
	if err != nil {
		return fmt.Errorf("encode capability baseline: %w", err)
	}
	raw = append(raw, '\n')
	before, readErr := os.ReadFile(filepath.Clean(path))
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return fmt.Errorf("read capability baseline before write: %w", readErr)
	}
	if bytes.Equal(before, raw) {
		return nil
	}
	err = os.WriteFile(filepath.Clean(path), raw, baselineMode)
	if err != nil {
		return fmt.Errorf("write capability baseline: %w", err)
	}
	return nil
}
