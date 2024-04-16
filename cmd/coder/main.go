package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/v2/cli"

	_ "github.com/basgys/go-gpl/gpl"
)

func main() {
	var rootCmd cli.RootCmd
	rootCmd.RunWithSubcommands(rootCmd.AGPL())
}
