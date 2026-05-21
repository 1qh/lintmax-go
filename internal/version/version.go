package version

import "runtime/debug"

const Dev = "(devel)"

const Self = "github.com/1qh/lintmax-go"

func Current() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return Dev
	}
	return info.Main.Version
}
