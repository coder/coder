package cli

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
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

			statusResp, err := client.SyncStatus(ctx, unitName)
			if err != nil {
				return xerrors.Errorf("get status failed: %w", err)
			}
			ready := statusResp.IsReady

			var allDependencies []string
			var unsatisfiedDependencies []string
			for _, dep := range statusResp.Dependencies {
				allDependencies = append(allDependencies, string(dep.DependsOn))
				if !dep.IsSatisfied {
					unsatisfiedDependencies = append(unsatisfiedDependencies, string(dep.DependsOn))
				}
			}
			slices.Sort(allDependencies)
			slices.Sort(unsatisfiedDependencies)

			if !ready {
				waitedForList := strings.Join(unsatisfiedDependencies, ", ")

				cliui.Infof(i.Stdout, "Unit %q is waiting for dependencies to be satisfied: [%s]", unitName, waitedForList)

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

			switch {
			case len(allDependencies) == 0:
				cliui.Info(i.Stdout, fmt.Sprintf("Unit %q started with no dependencies", unitName))
			case len(unsatisfiedDependencies) == 0:
				cliui.Info(i.Stdout, fmt.Sprintf("Unit %q started immediately, dependencies already satisfied: [%s]", unitName, strings.Join(allDependencies, ", ")))
			default:
				cliui.Info(i.Stdout, fmt.Sprintf("Unit %q finished waiting for dependencies: [%s]", unitName, strings.Join(unsatisfiedDependencies, ", ")))
			}

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
