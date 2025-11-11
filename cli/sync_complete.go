package cli

import (
	"context"
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func (r *RootCmd) syncComplete() *serpent.Command {
	return &serpent.Command{
		Use:   "complete <unit>",
		Short: "Mark a unit as complete in the dependency graph",
		Long:  "Set a unit's status to complete in the dependency graph.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := i.Args[0]

			// Show initial message
			fmt.Printf("Completing unit '%s'...\n", unit)

			// Connect to agent socket
			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			// Complete the unit
			if err := client.SyncComplete(ctx, unit); err != nil {
				return xerrors.Errorf("complete unit failed: %w", err)
			}

			// Display success message
			fmt.Printf("Unit '%s' completed successfully\n", unit)

			return nil
		},
	}
}
