package deadconst

import (
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

const nodeModules = "node_modules"

const loadMode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
	packages.NeedTypes | packages.NeedTypesInfo

type Issue struct {
	Pos  string
	Name string
	Pkg  string
}

func Scan(ctx context.Context, root string) ([]Issue, error) {
	var cfg packages.Config
	cfg.Mode = loadMode
	cfg.Tests = true
	cfg.Context = ctx
	cfg.Dir = root
	pkgs, err := packages.Load(&cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("deadconst load packages: %w", err)
	}
	used, candidates := index(pkgs)
	var issues []Issue
	for _, candidate := range candidates {
		if !used[candidate.Pos] {
			issues = append(issues, candidate)
		}
	}
	slices.SortFunc(issues, func(a, b Issue) int { return cmp.Compare(a.Pos, b.Pos) })
	return issues, nil
}

func index(pkgs []*packages.Package) (map[string]bool, []Issue) {
	used := map[string]bool{}
	var candidates []Issue
	for _, pkg := range pkgs {
		if pkg.TypesInfo == nil || pkg.Types == nil || strings.Contains(pkg.PkgPath, nodeModules) {
			continue
		}
		for _, obj := range pkg.TypesInfo.Uses {
			if constant, ok := obj.(*types.Const); ok {
				used[pkg.Fset.Position(constant.Pos()).String()] = true
			}
		}
		if pkg.ID == pkg.PkgPath && !strings.HasSuffix(pkg.PkgPath, ".test") {
			candidates = append(candidates, scanPkg(pkg)...)
		}
	}
	return used, candidates
}

func scanPkg(pkg *packages.Package) []Issue {
	var issues []Issue
	for _, file := range pkg.Syntax {
		path := pkg.Fset.Position(file.Pos()).Filename
		if strings.HasSuffix(path, "_test.go") || strings.Contains(path, nodeModules) || generated(file) {
			continue
		}
		for _, decl := range file.Decls {
			group, ok := decl.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST || usesIota(pkg, group) {
				continue
			}
			issues = append(issues, groupCandidates(pkg, group)...)
		}
	}
	return issues
}

func generated(file *ast.File) bool {
	if len(file.Comments) == 0 || len(file.Comments[0].List) == 0 {
		return false
	}
	comment := file.Comments[0].List[0]
	return comment.Pos() < file.Package && strings.HasPrefix(comment.Text, "// Code generated ") &&
		strings.HasSuffix(comment.Text, " DO NOT EDIT.")
}

func usesIota(pkg *packages.Package, group *ast.GenDecl) bool {
	found := false
	ast.Inspect(group, func(node ast.Node) bool {
		if ident, ok := node.(*ast.Ident); ok && pkg.TypesInfo.Uses[ident] == types.Universe.Lookup("iota") {
			found = true
		}
		return !found
	})
	return found
}

func groupCandidates(pkg *packages.Package, group *ast.GenDecl) []Issue {
	var issues []Issue
	for _, spec := range group.Specs {
		value, ok := spec.(*ast.ValueSpec)
		if !ok {
			continue
		}
		for _, name := range value.Names {
			constant, isConst := pkg.TypesInfo.Defs[name].(*types.Const)
			if !isConst || constant.Exported() || constant.Name() == "_" || constant.Parent() != pkg.Types.Scope() {
				continue
			}
			issues = append(issues, Issue{
				Pos:  pkg.Fset.Position(constant.Pos()).String(),
				Name: constant.Name(),
				Pkg:  pkg.PkgPath,
			})
		}
	}
	return issues
}

func Format(issues []Issue) string {
	var out strings.Builder
	for _, issue := range issues {
		out.WriteString(issue.Pos + ": " + issue.Name + " is declared and never read\n")
	}
	return out.String()
}
