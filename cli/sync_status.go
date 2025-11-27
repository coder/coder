package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/cli/cliui"
)

func (*RootCmd) syncStatus(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(
			cliui.TableFormat(
				[]agentsocket.DependencyInfo{},
				[]string{
					"depends on",
					"required status",
					"current status",
					"satisfied",
				},
			),
			func(data any) (any, error) {
				resp, ok := data.(agentsocket.SyncStatusResponse)
				if !ok {
					return nil, xerrors.Errorf("expected agentsocket.SyncStatusResponse, got %T", data)
				}
				return resp.Dependencies, nil
			}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "status <unit>",
		Short: "Show unit status and dependency state",
		Long:  "Show the current status of a unit, whether it is ready to start, and lists its dependencies. Shows which dependencies are satisfied and which are still pending. Supports multiple output formats.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			if len(i.Args) != 1 {
				return xerrors.New("exactly one unit name is required")
			}
			unit := unit.ID(i.Args[0])

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			statusResp, err := client.SyncStatus(ctx, unit)
			if err != nil {
				return xerrors.Errorf("get status failed: %w", err)
			}

			if formatter.FormatID() == "table" {
				header := fmt.Sprintf("Unit: %s\nStatus: %s\nReady: %t\n", unit, statusResp.Status, statusResp.IsReady)
				cliui.Infof(i.Stdout, "%s", header)
			}

			if len(statusResp.Dependencies) > 0 {
				out, err := formatter.Format(ctx, statusResp)
				if err != nil {
					return xerrors.Errorf("format status: %w", err)
				}

				_, _ = fmt.Fprintln(i.Stdout, out)
			}
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
