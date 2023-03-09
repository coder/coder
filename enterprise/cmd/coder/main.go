package main

import (
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	entcli "github.com/coder/coder/enterprise/cli"
)

func main() {
	cli.Main(entcli.EnterpriseSubcommands())
}
