package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) syncCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "sync",
		Short: "Manage unit dependencies for coordinated startup",
		Long:  "Commands for orchestrating unit startup order in workspaces. Units are most commonly coder scripts. Use these commands to declare dependencies between units, coordinate their startup sequence, and ensure units start only after their dependencies are ready. This helps to prevents race conditions and startup failures.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.syncPing(),
			r.syncStart(),
			r.syncWant(),
			r.syncComplete(),
			r.syncStatus(),
		},
	}
	return cmd
}
