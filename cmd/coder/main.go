package main

import (
	_ "time/tzdata"

	_ "github.com/coder/coder/v2/buildinfo/resources"
)

func main() {
	panic("hey")

	// This preserves backwards compatibility with an init function that is causing grief for
	// web terminals using agent-exec + screen. See https://github.com/coder/coder/pull/15817

	rootCmd.RunWithSubcommands(rootCmd.AGPL())
}
