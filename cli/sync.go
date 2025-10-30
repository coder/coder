package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) syncCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "sync",
		Short: "Synchronize with the local agent socket",
		Long:  "Commands for interacting with the local Coder agent via socket communication.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.syncPing(),
			r.syncStart(),
			r.syncWant(),
			r.syncComplete(),
			r.syncWait(),
			r.syncStatus(),
		},
	}
	return cmd
}
