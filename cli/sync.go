package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) syncCommand() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "sync",
		Short: "Manage service dependency graph for coordinated startup",
		Long:  "Commands for orchestrating service startup order in workspaces. Use these commands to declare dependencies between services, coordinate their startup sequence, and ensure services start only after their dependencies are ready. This prevents race conditions and startup failures in multi-service workspaces by managing a dependency graph that tracks service status and readiness.",
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
