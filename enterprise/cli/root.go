package cli

import (
	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
)

func enterpriseOnly() []*clibase.Command {
	return []*clibase.Command{
		server(),
		features(),
		licenses(),
		groups(),
		provisionerDaemons(),
	}
}

func EnterpriseSubcommands() []*clibase.Command {
	all := append(agpl.Core(), enterpriseOnly()...)
	return all
}
