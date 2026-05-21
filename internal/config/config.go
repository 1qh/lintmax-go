package config

import _ "embed"

//go:embed golangci.yml
var GolangCI []byte

//go:embed editorconfig.txt
var EditorConfig []byte

//go:embed consumer-ci.yml
var ConsumerCI []byte
