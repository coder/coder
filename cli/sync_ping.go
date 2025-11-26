package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncPing() *serpent.Command {
	return &serpent.Command{
		Use:   "ping",
		Short: "Verify agent connectivity and health",
		Long:  "Test connectivity to the local Coder agent socket to verify the agent is running and responsive. Useful for troubleshooting startup issues or verifying the agent is accessible before running other sync commands.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			client, err := agentsocket.NewClient(ctx)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			err = client.Ping(ctx)
			if err != nil {
				return xerrors.Errorf("ping failed: %w", err)
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}
}
