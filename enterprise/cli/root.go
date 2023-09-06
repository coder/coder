package cli

import (
	"github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/clibase"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*clibase.Cmd {
	return []*clibase.Cmd{
		r.Server(nil),
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
