package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func (r *RootCmd) syncPing() *serpent.Command {
	return &serpent.Command{
		Use:   "ping",
		Short: "Verify agent connectivity and health",
		Long:  "Test connectivity to the local Coder agent socket to verify the agent is running and responsive. Useful for troubleshooting startup issues or verifying the agent is accessible before running other sync commands.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			fmt.Println("Pinging agent socket...")

			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			start := time.Now()
			resp, err := client.Ping(ctx)
			duration := time.Since(start)

			if err != nil {
				return xerrors.Errorf("ping failed: %w", err)
			}

			fmt.Printf("Response: %s\n", resp.Message)
			fmt.Printf("Timestamp: %s\n", resp.Timestamp.Format(time.RFC3339))
			fmt.Printf("Round-trip time: %s\n", duration.Round(time.Microsecond))
			fmt.Println("Status: healthy")

			return nil
		},
	}
}
