package diag

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

type Diagnostic struct {
	File   string
	Linter string
	Rule   string
	Line   int
}

func key(d Diagnostic) string {
	return d.File + "|" + strconv.Itoa(d.Line) + "|" + canonical(d.Linter, d.Rule)
}

func canonical(linter, rule string) string {
	if rule == "" {
		return linter
	}
	return linter + "/" + rule
}

//nolint:tagliatelle // golangci-lint JSON output uses PascalCase keys (upstream wire shape)
type golangciPos struct {
	Filename string `json:"Filename"`
	Line     int    `json:"Line"`
}

//nolint:tagliatelle // golangci-lint JSON output uses PascalCase keys (upstream wire shape)
type golangciIssue struct {
	FromLinter string      `json:"FromLinter"`
	Text       string      `json:"Text"`
	Pos        golangciPos `json:"Pos"`
}

//nolint:tagliatelle // golangci-lint JSON output uses PascalCase keys (upstream wire shape)
type golangciOut struct {
	Issues []golangciIssue `json:"Issues"`
}

type analysisItem struct {
	Posn    string `json:"posn"`
	Message string `json:"message"`
}

func ruleFromText(text string) string {
	open := strings.LastIndex(text, "(")
	if open >= 0 && strings.HasSuffix(strings.TrimSpace(text), ")") {
		return strings.TrimSuffix(text[open+1:], ")")
	}
	colon := strings.Index(text, ":")
	if colon > 0 && colon < 12 && !strings.Contains(text[:colon], " ") {
		return text[:colon]
	}
	return ""
}

func ParseGolangci(raw []byte) []Diagnostic {
	var out golangciOut
	err := json.NewDecoder(bytes.NewReader(raw)).Decode(&out)
	if err != nil {
		return nil
	}
	diags := make([]Diagnostic, 0, len(out.Issues))
	for _, issue := range out.Issues {
		rule := ruleFromText(issue.Text)
		if rule == issue.FromLinter {
			rule = ""
		}
		diags = append(
			diags,
			Diagnostic{File: issue.Pos.Filename, Line: issue.Pos.Line, Linter: issue.FromLinter, Rule: rule},
		)
	}
	return diags
}

func ParseAnalysis(raw []byte, linter string) []Diagnostic {
	var diags []Diagnostic
	dec := json.NewDecoder(bytes.NewReader(raw))
	for {
		var byPkg map[string]map[string][]analysisItem
		derr := dec.Decode(&byPkg)
		if derr != nil {
			break
		}
		diags = append(diags, analysisDiags(byPkg, linter)...)
	}
	return diags
}

func analysisDiags(byPkg map[string]map[string][]analysisItem, linter string) []Diagnostic {
	var diags []Diagnostic
	for _, byAnalyzer := range byPkg {
		for analyzer, list := range byAnalyzer {
			for _, it := range list {
				f, l := splitPosn(it.Posn)
				diags = append(diags, Diagnostic{File: f, Line: l, Linter: linter, Rule: analyzer})
			}
		}
	}
	return diags
}

func ParseLines(raw []byte, linter string) []Diagnostic {
	const lineParts = 4
	var diags []Diagnostic
	for line := range strings.SplitSeq(string(raw), "\n") {
		parts := strings.SplitN(line, ":", lineParts)
		if len(parts) < lineParts {
			continue
		}
		num, err := strconv.Atoi(parts[1])
		if err != nil {
			continue
		}
		diags = append(diags, Diagnostic{File: parts[0], Line: num, Linter: linter, Rule: ""})
	}
	return diags
}

func splitPosn(posn string) (string, int) {
	const minPosnParts = 2
	parts := strings.Split(posn, ":")
	if len(parts) < minPosnParts {
		return posn, 0
	}
	num, err := strconv.Atoi(parts[1])
	if err != nil {
		return parts[0], 0
	}
	return parts[0], num
}

func resolvePath(p string) string {
	abs, err := filepath.Abs(p)
	if err != nil {
		return p
	}
	ev, evErr := filepath.EvalSymlinks(abs)
	if evErr == nil {
		return ev
	}
	return abs
}

func relpath(root, file string) string {
	rel, err := filepath.Rel(resolvePath(root), resolvePath(file))
	if err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(file)
}

func Outside(root, file string) bool {
	if file == "" || !filepath.IsAbs(file) {
		return false
	}
	return relOutside(root, file) && relOutside(resolvePath(root), resolvePath(file))
}

func relOutside(root, file string) bool {
	rel, err := filepath.Rel(root, file)
	if err != nil {
		return true
	}
	return rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func Format(diags []Diagnostic, root string) string {
	seen := map[string]bool{}
	byFile := map[string]map[string][]int{}
	for _, d := range diags {
		if seen[key(d)] {
			continue
		}
		seen[key(d)] = true
		f := relpath(root, d.File)
		if byFile[f] == nil {
			byFile[f] = map[string][]int{}
		}
		c := canonical(d.Linter, d.Rule)
		byFile[f][c] = append(byFile[f][c], d.Line)
	}
	files := make([]string, 0, len(byFile))
	for f := range byFile {
		files = append(files, f)
	}
	slices.Sort(files)
	var sb strings.Builder
	for _, f := range files {
		sb.WriteString(f)
		rules := byFile[f]
		names := make([]string, 0, len(rules))
		for r := range rules {
			names = append(names, r)
		}
		slices.Sort(names)
		for _, r := range names {
			lines := rules[r]
			slices.Sort(lines)
			sb.WriteString("  " + r + ":" + joinInts(lines))
		}
		sb.WriteString("\n")
	}
	return sb.String()
}

func joinInts(nums []int) string {
	parts := make([]string, len(nums))
	for i, n := range nums {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func Count(diags []Diagnostic) int {
	seen := map[string]bool{}
	for _, d := range diags {
		seen[key(d)] = true
	}
	return len(seen)
}
