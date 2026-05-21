// Package tools declares the external tools lintmax-go orchestrates.
// No version is ever pinned: every tool is fetched @latest so the setup never goes stale.
package tools

// Tool is one external binary lintmax-go installs and runs.
type Tool struct {
	// Name is the invoked binary name.
	Name string
	// Pkg is the go-installable package path (always resolved @latest).
	Pkg string
	// Why documents the strictness layer this tool covers.
	Why string
	// Deep marks slow tools that run only in the deep gate, not the fast inner loop.
	Deep bool
}

// All is the full orchestrated set. golangci-lint bundles ~115 linters at
// maintainer-compatible versions, so tracking it @latest keeps all of them fresh;
// the rest are tools golangci deliberately does not wrap.
//
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
		Why:  "nil-panic static analysis (TS-strict-null gap)",
		Deep: false,
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
}
