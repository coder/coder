package cli

import (
	"github.com/coder/serpent"
)

func (r *RootCmd) syncCommand() *serpent.Command {
	var socketPath string

	cmd := &serpent.Command{
		Use:   "sync",
		Short: "Manage unit dependencies for coordinated startup",
		Long:  "Commands for orchestrating unit startup order in workspaces. Units are most commonly coder scripts. Use these commands to declare dependencies between units, coordinate their startup sequence, and ensure units start only after their dependencies are ready. This helps prevent race conditions and startup failures.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Children: []*serpent.Command{
			r.syncPing(&socketPath),
			r.syncStart(&socketPath),
			r.syncWant(&socketPath),
			r.syncComplete(&socketPath),
			r.syncStatus(&socketPath),
		},
		Options: serpent.OptionSet{
			{
				Flag:        "socket-path",
				Env:         "CODER_AGENT_SOCKET_PATH",
				Description: "Specify the path for the agent socket.",
				Value:       serpent.StringOf(&socketPath),
			},
		},
	}

	// Add the socket path option to all child commands so it appears in their help
	cmd.Walk(func(c *serpent.Command) {
		// Skip the parent command itself
		if c == cmd {
			return
		}
		// Check if the option already exists (shouldn't, but be safe)
		existing := c.Options.ByName("socket-path")
		if existing == nil {
			c.Options = append(c.Options, serpent.Option{
				Flag:        "socket-path",
				Env:         "CODER_AGENT_SOCKET_PATH",
				Description: "Specify the path for the agent socket.",
				Value:       serpent.StringOf(&socketPath),
			})
		}
	})

	return cmd
}
