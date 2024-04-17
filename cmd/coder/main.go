package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/v2/cli"
)

func main() {
	var rootCmd cli.RootCmd
	rootCmd.RunWithSubcommands(rootCmd.AGPL())
}
