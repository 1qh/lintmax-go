package tools

type Tool struct {
	Name string
	Pkg  string
	Why  string
}

//nolint:gochecknoglobals // the tool registry is the package's data, inherently global
var All = []Tool{
	{
		Name: "golangci-lint",
		Pkg:  "github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		Why:  "115+ linters + formatters, default:all",
	},
	{
		Name: "nilaway",
		Pkg:  "go.uber.org/nilaway/cmd/nilaway",
		Why:  "nil-panic static analysis (TS-strict-null gap)",
	},
	{
		Name: "deadcode",
		Pkg:  "golang.org/x/tools/cmd/deadcode",
		Why:  "whole-program unreachable funcs",
	},
	{
		Name: "govulncheck",
		Pkg:  "golang.org/x/vuln/cmd/govulncheck",
		Why:  "reachability CVE scan",
	},
	{
		Name: "osv-scanner",
		Pkg:  "github.com/google/osv-scanner/v2/cmd/osv-scanner",
		Why:  "lockfile CVE scan",
	},
	{
		Name: "capslock",
		Pkg:  "github.com/google/capslock/cmd/capslock",
		Why:  "dependency capability analysis",
	},
	{
		Name: "gremlins",
		Pkg:  "github.com/go-gremlins/gremlins",
		Why:  "mutation testing (active 2026 alternative to avito-tech/go-mutesting)",
	},
}
