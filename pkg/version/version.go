// Package version reports the OCHQ build version.
package version

import "runtime/debug"

// Version is set at build time:
//
//	go build -ldflags "-X github.com/mukhiddinov/ochq/pkg/version.Version=v1.0.0" ./cmd/ochq
var Version = "dev"

func init() {
	if Version != "dev" {
		return
	}
	// Binaries built with `go install ...@v1.2.3` carry the module version.
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		Version = bi.Main.Version
	}
}
