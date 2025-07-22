package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) logs() *serpent.Command {
	var follow bool
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "logs <build-id>",
		Short:       "Show logs for a workspace build",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			buildIDStr := inv.Args[0]
			buildID, err := uuid.Parse(buildIDStr)
			if err != nil {
				return xerrors.Errorf("invalid build ID %q: %w", buildIDStr, err)
			}

			logs, closer, err := client.WorkspaceBuildLogsAfter(inv.Context(), buildID, 0)
			if err != nil {
				return xerrors.Errorf("get build logs: %w", err)
			}
			defer closer.Close()

			for {
				log, ok := <-logs
				if !ok {
					break
				}

				// Simple format with timestamp and stage
				timestamp := log.CreatedAt.Format("15:04:05")
				if log.Stage != "" {
					_, _ = fmt.Fprintf(inv.Stdout, "[%s] %s: %s\n",
						timestamp, log.Stage, log.Output)
				} else {
					_, _ = fmt.Fprintf(inv.Stdout, "[%s] %s\n",
						timestamp, log.Output)
				}
			}
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:          "follow",
			FlagShorthand: "f",
			Description:   "Follow log output (stream real-time logs).",
			Value:         serpent.BoolOf(&follow),
		},
	}

	return cmd
}
