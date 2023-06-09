package main

import (
	_ "time/tzdata"

	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	var rootCmd entcli.RootCmd
	rootCmd.RunMain(rootCmd.EnterpriseSubcommands())
}
