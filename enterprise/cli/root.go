package cli

import (
	"github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

type RootCmd struct {
	cli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*serpent.Cmd {
	return []*serpent.Cmd{
		r.Server(nil),
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.provisionerDaemons(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*serpent.Cmd {
	all := append(r.Core(), r.enterpriseOnly()...)
	return all
}
