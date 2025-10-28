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
		Short: "Ping the local agent socket",
		Long:  "Test connectivity to the local Coder agent via socket communication.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			// Show initial message
			fmt.Println("Pinging agent socket...")

			// Connect to agent socket
			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			// Measure round-trip time
			start := time.Now()
			resp, err := client.Ping(ctx)
			duration := time.Since(start)

			if err != nil {
				return xerrors.Errorf("ping failed: %w", err)
			}

			// Display results
			fmt.Printf("Response: %s\n", resp.Message)
			fmt.Printf("Timestamp: %s\n", resp.Timestamp.Format(time.RFC3339))
			fmt.Printf("Round-trip time: %s\n", duration.Round(time.Microsecond))
			fmt.Println("Status: healthy")

			return nil
		},
	}
}
