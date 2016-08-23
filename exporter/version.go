package exporter

import (
	"fmt"
	"runtime"
)

// The following strings are populated during build time with release.sh:
// go build -ldflags "-X importpath.name=value"
var (
	Version   string
	BuildDate string
	Branch    string
	Revision  string
	GoVersion = runtime.Version()
	Platform  = runtime.GOOS + "-" + runtime.GOARCH
)

func VersionString() string {
	return fmt.Sprintf("grok_exporter version: %v (build date: %v, branch: %v, revision: %v, go version: %v, platform: %v)", Version, BuildDate, Branch, Revision, GoVersion, Platform)
}
