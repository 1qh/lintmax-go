package config

import _ "embed"

//go:embed golangci.yml
var GolangCI []byte

//go:embed editorconfig.txt
var EditorConfig []byte

//go:embed consumer-ci.yml
var ConsumerCI []byte

//go:embed consumer-release.yml
var ConsumerRelease []byte

//go:embed prune-ci-runs.sh
var PruneCIRuns []byte

//go:embed prune-old-releases.sh
var PruneOldReleases []byte
