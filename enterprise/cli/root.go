package cli

import (
	"github.com/spf13/cobra"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/enterprise/coderd"
)

func enterpriseOnly() []*cobra.Command {
	return []*cobra.Command{
		agpl.Server(coderd.NewEnterprise),
		licenses(),
	}
}

func EnterpriseSubcommands() []*cobra.Command {
	all := append(agpl.Core(), enterpriseOnly()...)
	return all
}
