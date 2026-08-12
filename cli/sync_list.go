package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

func (*RootCmd) syncList(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]agentsocket.SyncListItem{},
			[]string{
				"unit",
				"status",
				"ready",
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "list",
		Short: "List all registered units and their statuses",
		Long:  "List all units currently registered with the workspace agent. Shows each unit's name, status, and whether it is ready to start.",
		Handler: func(i *serpent.Invocation) error {
			ctx := i.Context()

			opts := []agentsocket.Option{}
			if *socketPath != "" {
				opts = append(opts, agentsocket.WithPath(*socketPath))
			}

			client, err := agentsocket.NewClient(ctx, opts...)
			if err != nil {
				return xerrors.Errorf("connect to agent socket: %w", err)
			}
			defer client.Close()

			items, err := client.SyncList(ctx)
			if err != nil {
				return xerrors.Errorf("list units failed: %w", err)
			}

			if len(items) == 0 && formatter.FormatID() == "table" {
				cliui.Info(i.Stdout, "No units registered")
				return nil
			}

			out, err := formatter.Format(ctx, items)
			if err != nil {
				return xerrors.Errorf("format output: %w", err)
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
