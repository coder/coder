package cli

import (
	"encoding/json"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskLogs() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "logs <task>",
		Short: "Show a task's logs",
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

			enc := json.NewEncoder(inv.Stdout)
			for _, log := range logs.Logs {
				if err := enc.Encode(log); err != nil {
					return xerrors.Errorf("encode task log: %w", err)
				}
			}

			return nil
		},
	}

	return cmd
}
