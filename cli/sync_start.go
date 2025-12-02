package cli

import (
	"context"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
)

const (
	syncPollInterval = 1 * time.Second
)

func (*RootCmd) syncStart(socketPath *string) *serpent.Command {
	var timeout time.Duration

	cmd := &serpent.Command{
		Use:   "start <unit>",
		Short: "Wait until all unit dependencies are satisfied",
		Long:  "Wait until all dependencies are satisfied, consider the unit to have started, then allow it to proceed. This command polls until dependencies are ready, then marks the unit as started.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unitName := unit.ID(i.Args[0])

			if timeout > 0 {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, timeout)
				defer cancel()
			}

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			ready, err := client.SyncReady(ctx, unitName)
			if err != nil {
				return xerrors.Errorf("error checking dependencies: %w", err)
			}

			if !ready {
				cliui.Infof(i.Stdout, "Waiting for dependencies of unit '%s' to be satisfied...", unitName)

				ticker := time.NewTicker(syncPollInterval)
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
							break pollLoop
						}
					}
				}
			}

			if err := client.SyncStart(ctx, unitName); err != nil {
				return xerrors.Errorf("start unit failed: %w", err)
			}

			cliui.Info(i.Stdout, "Success")

			return nil
		},
	}

	cmd.Options = append(cmd.Options, serpent.Option{
		Flag:        "timeout",
		Description: "Maximum time to wait for dependencies (e.g., 30s, 5m). 5m by default.",
		Value:       serpent.DurationOf(&timeout),
		Default:     "5m",
	})

	return cmd
}
