package dupconst

import (
	"cmp"
	"context"
	"fmt"
	"go/constant"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/tools/go/packages"
)

const nodeModules = "node_modules"

const (
	minValueLen = 3
	minDupNames = 2
)

type Issue struct {
	Value string
	Pkg   string
	Names []string
}

func Scan(ctx context.Context, root string) ([]Issue, error) {
	var cfg packages.Config
	cfg.Mode = packages.NeedTypes | packages.NeedName | packages.NeedDeps | packages.NeedImports
	cfg.Context = ctx
	cfg.Dir = root
	pkgs, err := packages.Load(&cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("dupconst load packages: %w", err)
	}
	var issues []Issue
	for _, p := range pkgs {
		if p.Types == nil || strings.Contains(p.PkgPath, nodeModules) {
			continue
		}
		issues = append(issues, scanPkg(p.Types)...)
	}
	slices.SortFunc(issues, func(a, b Issue) int { return cmp.Compare(a.Value, b.Value) })
	return issues, nil
}

func scanPkg(pkg *types.Package) []Issue {
	byVal := map[string][]string{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		c, ok := scope.Lookup(name).(*types.Const)
		if !ok || c.Val().Kind() != constant.String {
			continue
		}
		v := constant.StringVal(c.Val())
		if len(v) < minValueLen {
			continue
		}
		byVal[c.Type().String()+"\x00"+v] = append(byVal[c.Type().String()+"\x00"+v], name)
	}
	var out []Issue
	for v, names := range byVal {
		if len(names) < minDupNames {
			continue
		}
		dispV := v
		if _, after, ok := strings.Cut(v, "\x00"); ok {
			dispV = after
		}
		for _, group := range byDomain(names) {
			slices.Sort(group)
			out = append(out, Issue{Value: dispV, Pkg: pkg.Name(), Names: group})
		}
	}
	return out
}

func byDomain(names []string) [][]string {
	used := make([]bool, len(names))
	var out [][]string
	for i, a := range names {
		if used[i] {
			continue
		}
		group := []string{a}
		for j := i + 1; j < len(names); j++ {
			if used[j] || !sameSubject(a, names[j]) {
				continue
			}
			used[j] = true
			group = append(group, names[j])
		}
		if len(group) >= minDupNames {
			used[i] = true
			out = append(out, group)
		}
	}
	slices.SortFunc(out, func(a, b []string) int { return cmp.Compare(a[0], b[0]) })
	return out
}

func sameSubject(a, b string) bool {
	ta, tb := tokens(a), tokens(b)
	if len(ta) > len(tb) {
		ta, tb = tb, ta
	}
	i := 0
	for _, t := range tb {
		if i < len(ta) && tokenAkin(ta[i], t) {
			i++
		}
	}
	return i == len(ta)
}

func tokenAkin(a, b string) bool {
	return strings.HasPrefix(a, b) || strings.HasPrefix(b, a)
}

func tokens(name string) []string {
	var out []string
	var cur []rune
	flush := func() {
		if len(cur) > 0 {
			out = append(out, strings.ToLower(string(cur)))
			cur = nil
		}
	}
	runes := []rune(name)
	for i, r := range runes {
		if r == '_' {
			flush()
			continue
		}
		if startsToken(runes, i) {
			flush()
		}
		cur = append(cur, r)
	}
	flush()
	return out
}

func Format(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	for _, is := range issues {
		b.WriteString("  " + strings.Join(is.Names, " = ") + " = " + strconv.Quote(is.Value) +
			" — duplicate-value consts; collapse to one\n")
	}
	return b.String()
}

func startsToken(runes []rune, i int) bool {
	if i == 0 || !unicode.IsUpper(runes[i]) {
		return false
	}
	if !unicode.IsUpper(runes[i-1]) {
		return true
	}
	return i+1 < len(runes) && !unicode.IsUpper(runes[i+1])
}
