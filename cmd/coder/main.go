package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/cli"
)

func main() {
	var rootCmd cli.RootCmd
	rootCmd.RunMain(rootCmd.AGPL())
}
