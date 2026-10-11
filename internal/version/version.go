package version

import "runtime/debug"

const Release = "v0.25.12"

const Dev = Release + " (devel)"

const Self = "github.com/1qh/lintmax-go"

func Current() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return Dev
	}
	return info.Main.Version
}
