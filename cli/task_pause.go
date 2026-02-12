package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskPause() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "pause <task>",
		Short: "Pause a task",
		Long: FormatExamples(
			Example{
				Description: "Pause a task by name",
				Command:     "coder task pause my-task",
			},
			Example{
				Description: "Pause another user's task",
				Command:     "coder task pause alice/my-task",
			},
			Example{
				Description: "Pause a task without confirmation",
				Command:     "coder task pause my-task --yes",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
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

			task, err := client.TaskByIdentifier(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("resolve task %q: %w", inv.Args[0], err)
			}

			display := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)

			if task.Status == codersdk.TaskStatusPaused {
				return xerrors.Errorf("task %q is already paused", display)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Pause task %s?", pretty.Sprint(cliui.DefaultStyles.Code, display)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			resp, err := client.PauseTask(ctx, task.OwnerName, task.ID)
			if err != nil {
				return xerrors.Errorf("pause task %q: %w", display, err)
			}

			if resp.WorkspaceBuild == nil {
				return xerrors.Errorf("pause task %q: no workspace build returned", display)
			}

			err = cliui.WorkspaceBuild(ctx, inv.Stdout, client, resp.WorkspaceBuild.ID)
			if err != nil {
				return xerrors.Errorf("watch pause build for task %q: %w", display, err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s task has been paused at %s!\n",
				cliui.Keyword(task.Name),
				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	return cmd
}
