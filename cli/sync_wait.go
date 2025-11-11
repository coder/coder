package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

const (
	// PollInterval is the interval between dependency checks
	PollInterval = 1 * time.Second
)

func (r *RootCmd) syncWait() *serpent.Command {
	var timeout time.Duration

	cmd := &serpent.Command{
		Use:   "wait <unit>",
		Short: "Wait for a unit's dependencies to be satisfied",
		Long:  "Poll until all dependencies for a unit are met. Exits when dependencies are satisfied or timeout is reached.",
		Handler: func(i *serpent.Invocation) error {
			ctx := context.Background()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unitName := i.Args[0]

			// Set up context with timeout if specified
			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			// Show initial message
			fmt.Printf("Waiting for dependencies of unit '%s' to be satisfied...\n", unitName)

			// Connect to agent socket
			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			// Poll until dependencies are satisfied
			ticker := time.NewTicker(PollInterval)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					if ctx.Err() == context.DeadlineExceeded {
						return xerrors.Errorf("timeout waiting for dependencies of unit '%s'", unitName)
					}
					return ctx.Err()
				case <-ticker.C:
					// Check if dependencies are satisfied
					err := client.SyncReady(ctx, unitName)
					if err == nil {
						// Dependencies are satisfied
						fmt.Printf("Dependencies for unit '%s' are now satisfied\n", unitName)
						return nil
					}

					// Check if it's a "not ready" error (expected while waiting)
					if xerrors.Is(err, unit.ErrDependenciesNotSatisfied) {
						// Still waiting, continue polling
						continue
					}

					// Some other error occurred
					return xerrors.Errorf("error checking dependencies: %w", err)
				}
			}
		},
	}

	cmd.Options = append(cmd.Options, serpent.Option{
		Flag:        "timeout",
		Description: "Maximum time to wait for dependencies (e.g., 30s, 5m). No timeout by default.",
		Value:       serpent.DurationOf(&timeout),
	})

	return cmd
}
