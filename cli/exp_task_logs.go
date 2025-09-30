package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskLogs() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat(
			[]codersdk.TaskLogEntry{},
			[]string{
				"type",
				"content",
			},
		),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "logs <task>",
		Short: "Show a task's logs",
		Long: `# Show a task's logs.
$ coder exp task logs task1`,
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var (
				ctx    = inv.Context()
				exp    = codersdk.NewExperimentalClient(client)
				task   = inv.Args[0]
				taskID uuid.UUID
			)

			if id, err := uuid.Parse(task); err == nil {
				taskID = id
			} else {
				ws, err := namedWorkspace(ctx, client, task)
				if err != nil {
					return xerrors.Errorf("resolve task %q: %w", task, err)
				}

				taskID = ws.ID
			}

			logs, err := exp.TaskLogs(ctx, codersdk.Me, taskID)
			if err != nil {
				return xerrors.Errorf("get task logs: %w", err)
			}

			out, err := formatter.Format(ctx, logs.Logs)
			if err != nil {
				return xerrors.Errorf("format task logs: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
