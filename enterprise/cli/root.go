package cli

import (
	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*clibase.Cmd {
	return []*clibase.Cmd{
		r.server(),
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.provisionerDaemons(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*clibase.Cmd {
	all := append(r.Core(), r.enterpriseOnly()...)
	return all
}
