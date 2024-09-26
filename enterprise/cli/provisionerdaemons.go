package cli

import "github.com/coder/serpent"

func (r *RootCmd) provisionerDaemons() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "provisioner",
		Short: "Manage provisioner daemons",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Aliases: []string{"provisioners"},
		Children: []*serpent.Command{
			r.provisionerDaemonStart(),
			r.provisionerKeys(),
		},
	}

	return cmd
}

// The provisionerd command group is deprecated and hidden but kept around
// for backwards compatibility.
func (r *RootCmd) provisionerd() *serpent.Command {
	cmd := r.provisionerDaemons()
	cmd.Use = "provisionerd"
	cmd.Hidden = true

	return cmd
}
