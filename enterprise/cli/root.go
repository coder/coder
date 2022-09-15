package cli

import (
	"github.com/spf13/cobra"

	agpl "github.com/coder/coder/cli"
)

func enterpriseOnly() []*cobra.Command {
	return []*cobra.Command{
		server(),
		features(),
		licenses(),
	}
}

func EnterpriseSubcommands() []*cobra.Command {
	all := append(agpl.Core(), enterpriseOnly()...)
	return all
}
