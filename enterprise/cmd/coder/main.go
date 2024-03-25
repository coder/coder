package main

import (
	_ "time/tzdata"

	entcli "github.com/coder/coder/v2/enterprise/cli"
)

func main() {
	var rootCmd entcli.RootCmd
	rootCmd.RunWithSubcommands(rootCmd.EnterpriseSubcommands())
}
