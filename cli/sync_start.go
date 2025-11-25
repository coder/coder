package cli

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
)

const (
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

			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			fmt.Printf("Starting unit '%s'...\n", unitName)

			client, err := agentsocket.NewClient(ctx, "")
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			ready, err := client.SyncReady(ctx, unitName)
			if err != nil {
				return xerrors.Errorf("error checking dependencies: %w", err)
			}

			if !ready {
				fmt.Printf("Waiting for dependencies of unit '%s' to be satisfied...\n", unitName)

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
						ready, err := client.SyncReady(ctx, unitName)
						if err != nil {
							return xerrors.Errorf("error checking dependencies: %w", err)
						}
						if ready {
							fmt.Printf("Dependencies satisfied, marking unit '%s' as started\n", unitName)
							break pollLoop
						}
					}
				}
			} else {
				fmt.Printf("Dependencies satisfied, marking unit '%s' as started\n", unitName)
			}

			if err := client.SyncStart(ctx, unitName); err != nil {
				return xerrors.Errorf("start unit failed: %w", err)
			}

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
