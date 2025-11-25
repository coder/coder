package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/serpent"
)

func (r *RootCmd) syncPing() *serpent.Command {
	return &serpent.Command{
		Use:   "ping",
		Short: "Verify agent connectivity and health",
		Long:  "Test connectivity to the local Coder agent socket to verify the agent is running and responsive. Useful for troubleshooting startup issues or verifying the agent is accessible before running other sync commands.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			fmt.Println("Pinging agent socket...")

			client, err := agentsocket.NewClient(ctx, "")
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			start := time.Now()
			err = client.Ping(ctx)
			duration := time.Since(start)

			if err != nil {
				return xerrors.Errorf("ping failed: %w", err)
			}

			fmt.Printf("Round-trip time: %s\n", duration.Round(time.Microsecond))

			return nil
		},
	}
}
