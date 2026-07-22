package caps

import (
	"encoding/json"
	"slices"
	"strings"
)

const pairSep = "|"

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
	b.WriteString("  (re-run to accept the new set as the baseline once the change is understood)")
	return b.String()
}
