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

	"golang.org/x/tools/go/packages"
)

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
	cfg := &packages.Config{
		Mode:    packages.NeedTypes | packages.NeedName | packages.NeedDeps | packages.NeedImports,
		Dir:     root,
		Context: ctx,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("dupconst load packages: %w", err)
	}
	var issues []Issue
	for _, p := range pkgs {
		if p.Types == nil {
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
		byVal[v] = append(byVal[v], name)
	}
	var out []Issue
	for v, names := range byVal {
		if len(names) < minDupNames {
			continue
		}
		slices.Sort(names)
		out = append(out, Issue{Value: v, Pkg: pkg.Name(), Names: names})
	}
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
