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
	// SyncPollInterval is the interval between dependency checks for sync start
	SyncPollInterval = 1 * time.Second
)

func (r *RootCmd) syncStart() *serpent.Command {
	var timeout time.Duration

	cmd := &serpent.Command{
		Use:   "start <unit>",
		Short: "Start a service and wait for dependencies",
		Long:  "Start a service unit and automatically wait for all its dependencies to be satisfied before proceeding. This command registers the unit in the dependency graph, polls until dependencies are ready, then marks the unit as started. Use this as the primary command for starting services in a coordinated sequence.",
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
			fmt.Printf("Starting unit '%s'...\n", unitName)

			// Connect to agent socket
			client, err := agentsdk.NewSocketClient(agentsdk.SocketConfig{
				Path: "/tmp/coder.sock",
			})
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			// Check if dependencies are satisfied first
			err = client.SyncReady(ctx, unitName)
			if err != nil {
				// Check if it's a "not ready" error (expected if dependencies exist)
				if xerrors.Is(err, unit.ErrDependenciesNotSatisfied) {
					// Dependencies exist but aren't satisfied, start polling
					fmt.Printf("Waiting for dependencies of unit '%s' to be satisfied...\n", unitName)

					// Poll until dependencies are satisfied
					ticker := time.NewTicker(SyncPollInterval)
					defer ticker.Stop()

				pollLoop:
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
								fmt.Printf("Dependencies satisfied, marking unit '%s' as started\n", unitName)
								break pollLoop
							}

							// Check if it's still a "not ready" error (expected while waiting)
							if xerrors.Is(err, unit.ErrDependenciesNotSatisfied) {
								// Still waiting, continue polling
								continue
							}

							// Some other error occurred
							return xerrors.Errorf("error checking dependencies: %w", err)
						}
					}
				} else {
					// Some other error occurred
					return xerrors.Errorf("error checking dependencies: %w", err)
				}
			} else {
				// No dependencies or already satisfied
				fmt.Printf("Dependencies satisfied, marking unit '%s' as started\n", unitName)
			}

			// Start the unit
			if err := client.SyncStart(ctx, unitName); err != nil {
				return xerrors.Errorf("start unit failed: %w", err)
			}

			// Display success message
			fmt.Printf("Unit '%s' started successfully\n", unitName)

			return nil
		},
	}

	cmd.Options = append(cmd.Options, serpent.Option{
		Flag:        "timeout",
		Description: "Maximum time to wait for dependencies (e.g., 30s, 5m). No timeout by default.",
		Value:       serpent.DurationOf(&timeout),
	})

	return cmd
}
