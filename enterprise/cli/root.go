package cli

import (
	agplcli "github.com/coder/coder/v2/cli"
	"github.com/coder/serpent"
)

type RootCmd struct {
	agplcli.RootCmd
}

func (r *RootCmd) enterpriseOnly() []*serpent.Command {
	return []*serpent.Command{
		// These commands exist in AGPL, but we use a different implementation
		// in enterprise:
		r.Server(nil),
		r.provisionerDaemons(),
		agplcli.ExperimentalCommand(append(r.AGPLExperimental(), r.enterpriseExperimental()...)),

		// New commands that don't exist in AGPL:
		r.workspaceProxy(),
		r.features(),
		r.licenses(),
		r.groups(),
		r.prebuilds(),
		r.provisionerd(),
		r.externalWorkspaces(),
	}
}

func (r *RootCmd) enterpriseExperimental() []*serpent.Command {
	return []*serpent.Command{
		r.aibridge(),
	}
}

func (r *RootCmd) EnterpriseSubcommands() []*serpent.Command {
	all := append(r.CoreSubcommands(), r.enterpriseOnly()...)
	return all
}
