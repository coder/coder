package cli

import (
	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
)

func enterpriseOnly() []*clibase.Cmd {
	return []*clibase.Cmd{
		server(),
		features(),
		licenses(),
		groups(),
		provisionerDaemons(),
	}
}

func EnterpriseSubcommands() []*clibase.Cmd {
	all := append(agpl.Core(), enterpriseOnly()...)
	return all
}
