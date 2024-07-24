package cli

import "github.com/coder/serpent"

func (r *RootCmd) provisionerDaemons() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "provisionerd",
		Short: "Manage provisioner daemons",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"provisioner"},
		Children: []*serpent.Command{
			r.provisionerDaemonStart(),
			r.provisionerKeys(),
		},
	}

	return cmd
}
