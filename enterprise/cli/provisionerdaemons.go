package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) provisionerDaemons() *serpent.Command {
	cmd := r.RootCmd.Provisioners()
	cmd.AddSubcommands(
		r.provisionerDaemonStart(),
		r.provisionerKeys(),
	)

	return cmd
}

// The provisionerd command group is deprecated and hidden but kept around
// for backwards compatibility with the start command.
func (r *RootCmd) provisionerd() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.provisionerDaemonStart(),
		},
		Hidden: true,
	}

	return cmd
}
