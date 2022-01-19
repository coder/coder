package buildmode

import (
	"flag"
	"strings"
)

// BuildMode is injected at build time.
var (
	BuildMode string
)

// Dev returns true when built to run in a dev deployment.
func Dev() bool {
	return strings.HasPrefix(BuildMode, "dev")
}

// Test returns true when running inside a unit test.
func Test() bool {
	return flag.Lookup("test.v") != nil
}
