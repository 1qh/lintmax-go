// Package config embeds the maxed golangci-lint configuration.
package config

import _ "embed"

// GolangCI is the embedded maximum-strictness golangci-lint v2 config.
// Shipped inside lintmax-go so every consumer gets identical strictness
// by version, never a copy-pasted .golangci.yml that drifts.
//
//go:embed golangci.yml
var GolangCI []byte
