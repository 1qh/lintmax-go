package floatdiv

import (
	"bytes"
	"cmp"
	"context"
	"fmt"
	"go/ast"
	"go/printer"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/packages"
)

const nodeModules = "node_modules"

const (
	fnLen     = "len"
	fnLenCall = "len("
)

const loadMode = packages.NeedTypes |
	packages.NeedSyntax |
	packages.NeedTypesInfo |
	packages.NeedName |
	packages.NeedFiles

type Issue struct {
	Pos     string
	Operand string
	Func    string
}

func Scan(ctx context.Context, root string) ([]Issue, error) {
	cfg := &packages.Config{Mode: loadMode, Dir: root, Context: ctx}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return nil, fmt.Errorf("floatdiv load packages: %w", err)
	}
	var issues []Issue
	for _, p := range pkgs {
		if p.TypesInfo == nil || strings.Contains(p.PkgPath, nodeModules) {
			continue
		}
		for _, f := range p.Syntax {
			issues = append(issues, scanFile(p, f)...)
		}
	}
	slices.SortFunc(issues, func(a, b Issue) int { return cmp.Compare(a.Pos, b.Pos) })
	return issues, nil
}

func scanFile(p *packages.Package, f *ast.File) []Issue {
	var out []Issue
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if ok && fn.Body != nil {
			out = append(out, scanFunc(p, fn)...)
		}
		return true
	})
	return out
}

func scanFunc(p *packages.Package, fn *ast.FuncDecl) []Issue {
	guarded := guardedOperands(fn.Body)
	var out []Issue
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		is, ok := riskyDivision(p, fn, guarded, n)
		if ok {
			out = append(out, is)
		}
		return true
	})
	return out
}

func riskyDivision(
	p *packages.Package,
	fn *ast.FuncDecl,
	guarded map[string]struct{},
	n ast.Node,
) (Issue, bool) {
	bin, ok := n.(*ast.BinaryExpr)
	if !ok || bin.Op != token.QUO || !isFloat(p.TypesInfo.TypeOf(bin)) {
		return Issue{}, false //nolint:exhaustruct // zero sentinel ignored when ok=false
	}
	den, risky := riskyDenominator(bin.Y)
	if !risky {
		return Issue{}, false //nolint:exhaustruct // zero sentinel ignored when ok=false
	}
	if _, found := guarded[den]; found {
		return Issue{}, false //nolint:exhaustruct // zero sentinel ignored when ok=false
	}
	return Issue{Pos: p.Fset.Position(bin.Pos()).String(), Operand: den, Func: fn.Name.Name}, true
}

func riskyDenominator(y ast.Expr) (string, bool) {
	arg, ok := floatConvArg(y)
	if !ok {
		return "", false
	}
	if name, lok := lenCallText(arg); lok {
		return name, true
	}
	if id, idok := arg.(*ast.Ident); idok {
		return id.Name, true
	}
	return "", false
}

func floatConvArg(y ast.Expr) (ast.Expr, bool) {
	call, ok := y.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil, false
	}
	name, ok := call.Fun.(*ast.Ident)
	if !ok || (name.Name != "float64" && name.Name != "float32") {
		return nil, false
	}
	return call.Args[0], true
}

func lenCallText(e ast.Expr) (string, bool) {
	call, ok := e.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return "", false
	}
	id, ok := call.Fun.(*ast.Ident)
	if !ok || id.Name != fnLen {
		return "", false
	}
	return fnLenCall + render(call.Args[0]) + ")", true
}

func guardedOperands(body *ast.BlockStmt) map[string]struct{} {
	guarded := map[string]struct{}{}
	ast.Inspect(body, func(n ast.Node) bool {
		bin, ok := n.(*ast.BinaryExpr)
		if !ok || !isComparison(bin.Op) {
			return true
		}
		markGuard(bin.X, guarded)
		markGuard(bin.Y, guarded)
		return true
	})
	return guarded
}

func isComparison(op token.Token) bool {
	//nolint:exhaustive // reason: only the comparison ops matter; default covers the rest
	switch op {
	case token.EQL, token.NEQ, token.GTR, token.LSS, token.GEQ, token.LEQ:
		return true
	default:
		return false
	}
}

func markGuard(e ast.Expr, guarded map[string]struct{}) {
	if call, ok := e.(*ast.CallExpr); ok {
		id, idok := call.Fun.(*ast.Ident)
		if idok && id.Name == fnLen && len(call.Args) == 1 {
			guarded[fnLenCall+render(call.Args[0])+")"] = struct{}{}
		}
		return
	}
	if id, ok := e.(*ast.Ident); ok {
		guarded[id.Name] = struct{}{}
	}
}

func isFloat(t types.Type) bool {
	if t == nil {
		return false
	}
	b, ok := t.Underlying().(*types.Basic)
	return ok && b.Info()&types.IsFloat != 0
}

func render(e ast.Expr) string {
	var b bytes.Buffer
	err := printer.Fprint(&b, token.NewFileSet(), e)
	if err != nil {
		return ""
	}
	return b.String()
}

func Format(issues []Issue) string {
	if len(issues) == 0 {
		return ""
	}
	var b strings.Builder
	for _, is := range issues {
		b.WriteString("  " + is.Pos + " (" + is.Func + "): float division by " + is.Operand +
			" with no zero/empty guard — NaN/Inf on empty input breaks json.Marshal\n")
	}
	return b.String()
}
