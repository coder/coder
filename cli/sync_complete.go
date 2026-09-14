package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncComplete(socketPath *string) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "complete <unit>",
		Short: "Mark a unit as complete",
		Long:  "Mark a unit as complete. Indicating to other units that it has completed its work. This allows units that depend on it to proceed with their startup.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := unit.ID(i.Args[0])

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			if err := client.SyncComplete(ctx, unit); err != nil {
				return xerrors.Errorf("complete unit failed: %w", err)
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}

	return cmd
}
