package cli

import (
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskSend() *serpent.Command {
	var stdin bool

	cmd := &serpent.Command{
		Use:   "send <task> [<input> | --stdin]",
		Short: "Send input to a task",
		Long: FormatExamples(Example{
			Description: "Send direct input to a task.",
			Command:     "coder task send task1 \"Please also add unit tests\"",
		}, Example{
			Description: "Send input from stdin to a task.",
			Command:     "echo \"Please also add unit tests\" | coder task send task1 --stdin",
		}),
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
				ctx        = inv.Context()
				exp        = codersdk.NewExperimentalClient(client)
				identifier = inv.Args[0]

				taskInput string
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

			task, err := exp.TaskByIdentifier(ctx, identifier)
			if err != nil {
				return xerrors.Errorf("resolve task: %w", err)
			}

			if err = exp.TaskSend(ctx, codersdk.Me, task.ID, codersdk.TaskSendRequest{Input: taskInput}); err != nil {
				return xerrors.Errorf("send input to task: %w", err)
			}

			return nil
		},
	}

	return cmd
}
