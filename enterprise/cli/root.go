package cli

import (
	"github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*serpent.Command {
	return []*serpent.Command{
		r.Server(nil),
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.provisionerDaemons(),
		r.roles(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*serpent.Command {
	all := append(r.CoreSubcommands(), r.enterpriseOnly()...)
	return all
}
