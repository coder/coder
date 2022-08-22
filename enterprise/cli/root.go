package cli

import (
	"github.com/spf13/cobra"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/enterprise/coderd"
)

func EnterpriseSubcommands() []*cobra.Command {
	all := append(agpl.Core(), agpl.Server(coderd.NewEnterprise))
	return all
}
