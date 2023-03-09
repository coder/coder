package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	var rootCmd entcli.RootCmd

	cli.Main(rootCmd.EnterpriseSubcommands())
}
