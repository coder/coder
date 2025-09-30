package cli

import (
	"io"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskSend() *serpent.Command {
	var stdin bool

	cmd := &serpent.Command{
		Use:   "send <task> [<input> | --stdin]",
		Short: "Send input to a task",
		Long: `# Send input to a task.
$ coder exp task send task1 "Please also add unit tests"

# Send input from stdin to a task.
$ echo "Please also add unit tests" | coder exp task send task1 --stdin`,
		Middleware: serpent.RequireRangeArgs(1, 2),
		Options: serpent.OptionSet{
			{
				Name:        "stdin",
				Flag:        "stdin",
				Description: "Reads the input from stdin.",
				Value:       serpent.BoolOf(&stdin),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var (
				ctx  = inv.Context()
				exp  = codersdk.NewExperimentalClient(client)
				task = inv.Args[0]

				taskInput string
				taskID    uuid.UUID
			)

			if stdin {
				bytes, err := io.ReadAll(inv.Stdin)
				if err != nil {
					return xerrors.Errorf("reading stdio: %w", err)
				}

				taskInput = string(bytes)
			} else {
				if len(inv.Args) != 2 {
					return xerrors.Errorf("expected an input for the task")
				}

				taskInput = inv.Args[1]
			}

			if id, err := uuid.Parse(task); err == nil {
				taskID = id
			} else {
				ws, err := namedWorkspace(ctx, client, task)
				if err != nil {
					return xerrors.Errorf("resolve task: %w", err)
				}

				taskID = ws.ID
			}

			if err = exp.TaskSend(ctx, codersdk.Me, taskID, codersdk.TaskSendRequest{Input: taskInput}); err != nil {
				return xerrors.Errorf("send input to task: %w", err)
			}

			return nil
		},
	}

	return cmd
}
