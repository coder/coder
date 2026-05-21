package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncPing(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "ping",
		Short: "Test agent socket connectivity and health",
		Long:  "Test connectivity to the local Coder agent socket to verify the agent is running and responsive. Useful for troubleshooting startup issues or verifying the agent is accessible before running other sync commands.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
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

	return cmd
}
