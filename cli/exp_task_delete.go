package cli

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskDelete() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "delete <task> [<task> ...]",
		Short: "Delete experimental tasks",
		Long: FormatExamples(
			Example{
				Description: "Delete a single task.",
				Command:     "$ coder exp task delete task1",
			},
			Example{
				Description: "Delete multiple tasks.",
				Command:     "$ coder exp task delete task1 task2 task3",
			},
			Example{
				Description: "Delete a task without confirmation.",
				Command:     "$ coder exp task delete task4 --yes",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(1, -1),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			exp := codersdk.NewExperimentalClient(client)

			var tasks []codersdk.Task
			for _, identifier := range inv.Args {
				task, err := exp.TaskByIdentifier(ctx, identifier)
				if err != nil {
					return xerrors.Errorf("resolve task %q: %w", identifier, err)
				}
				tasks = append(tasks, task)
			}

			// Confirm deletion of the tasks.
			var displayList []string
			for _, task := range tasks {
				displayList = append(displayList, fmt.Sprintf("%s/%s", task.OwnerName, task.Name))
			}
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete these tasks: %s?", pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(displayList, ", "))),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			for i, task := range tasks {
				display := displayList[i]
				if err := exp.DeleteTask(ctx, task.OwnerName, task.ID); err != nil {
					return xerrors.Errorf("delete task %q: %w", display, err)
				}
				_, _ = fmt.Fprintln(
					inv.Stdout, "Deleted task "+pretty.Sprint(cliui.DefaultStyles.Keyword, display)+" at "+cliui.Timestamp(time.Now()),
				)
			}

			return nil
		},
	}

	return cmd
}
