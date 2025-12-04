package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent/agentsocket"
	"github.com/coder/coder/v2/cli/cliui"
)

func (*RootCmd) syncList(socketPath *string) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]agentsocket.ScriptInfo{},
			[]string{
				"id",
				"status",
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "list",
		Short: "List all units in the dependency graph",
		Long:  "List all units registered in the dependency graph, including their current status. Units can be coder scripts or other units registered via sync commands.",
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

			scripts, err := client.SyncList(ctx)
			if err != nil {
				return xerrors.Errorf("list scripts failed: %w", err)
			}

			out, err := formatter.Format(ctx, scripts)
			if err != nil {
				return xerrors.Errorf("format scripts: %w", err)
			}

			_, _ = fmt.Fprintln(i.Stdout, out)

			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

