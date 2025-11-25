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
		Short: "Signal that a service has finished",
		Long:  "Mark a service unit as complete, indicating it has finished its work and is ready. This allows dependent units that are waiting for this unit to proceed with their startup. Call this after a service has completed its startup and is ready to accept connections or requests.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := i.Args[0]

			fmt.Printf("Completing unit '%s'...\n", unit)

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			if err := client.SyncComplete(ctx, unit); err != nil {
				return xerrors.Errorf("complete unit failed: %w", err)
			}

			fmt.Printf("Unit '%s' completed successfully\n", unit)

			return nil
		},
	}
}
