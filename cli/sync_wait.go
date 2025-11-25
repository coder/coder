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
	PollInterval = 1 * time.Second
)

func (r *RootCmd) syncWait() *serpent.Command {
	var timeout time.Duration

	cmd := &serpent.Command{
		Use:   "wait <unit>",
		Short: "Wait for dependencies without starting the unit",
		Long:  "Poll until all dependencies for a unit are satisfied, then exit. Unlike 'start', this command does not mark the unit as started - it only waits. Useful for scripts that need to wait for dependencies but handle service startup themselves, or for synchronizing external processes with the dependency graph.",
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

			fmt.Printf("Waiting for dependencies of unit '%s' to be satisfied...\n", unitName)

			client, err := agentsocket.NewClient(ctx, "")
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

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
					ready, err := client.SyncReady(ctx, unitName)
					if err != nil {
						return xerrors.Errorf("error checking dependencies: %w", err)
					}
					if ready {
						fmt.Printf("Dependencies for unit '%s' are now satisfied\n", unitName)
						return nil
					}
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
