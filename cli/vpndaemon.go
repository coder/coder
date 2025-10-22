package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) vpnDaemon() *serpent.Command {
	cmd := &serpent.Command{
		Use:    "vpn-daemon [subcommand]",
		Short:  "VPN daemon commands used by Coder Desktop.",
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.vpnDaemonRun(),
		},
	}

	return cmd
}
