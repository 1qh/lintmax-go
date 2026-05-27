package tools

type Tool struct {
	Name string
	Pkg  string
	Why  string
	Deep bool
}

//nolint:gochecknoglobals // the tool registry is the package's data, inherently global
var All = []Tool{
	{
		Name: "golangci-lint",
		Pkg:  "github.com/golangci/golangci-lint/v2/cmd/golangci-lint",
		Why:  "115+ linters + formatters, default:all",
		Deep: false,
	},
	{
		Name: "nilaway",
		Pkg:  "go.uber.org/nilaway/cmd/nilaway",
		Why:  "nil-panic static analysis (TS-strict-null gap); --deep only since v0.18.0",
		Deep: true,
	},
	{
		Name: "deadcode",
		Pkg:  "golang.org/x/tools/cmd/deadcode",
		Why:  "whole-program unreachable funcs",
		Deep: false,
	},
	{
		Name: "govulncheck",
		Pkg:  "golang.org/x/vuln/cmd/govulncheck",
		Why:  "reachability CVE scan",
		Deep: true,
	},
	{
		Name: "osv-scanner",
		Pkg:  "github.com/google/osv-scanner/v2/cmd/osv-scanner",
		Why:  "lockfile CVE scan",
		Deep: true,
	},
	{
		Name: "capslock",
		Pkg:  "github.com/google/capslock/cmd/capslock",
		Why:  "dependency capability analysis",
		Deep: true,
	},
}
