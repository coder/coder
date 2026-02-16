package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskResume() *serpent.Command {
	var noWait bool

	cmd := &serpent.Command{
		Use:   "resume <task>",
		Short: "Resume a task",
		Long: FormatExamples(
			Example{
				Description: "Resume a task by name",
				Command:     "coder task resume my-task",
			},
			Example{
				Description: "Resume another user's task",
				Command:     "coder task resume alice/my-task",
			},
			Example{
				Description: "Resume a task without confirmation",
				Command:     "coder task resume my-task --yes",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Flag:        "no-wait",
				Description: "Return immediately after resuming the task.",
				Value:       serpent.BoolOf(&noWait),
			},
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

			if task.Status != codersdk.TaskStatusPaused {
				return xerrors.Errorf("task %q is not paused (current status: %s)", display, task.Status)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Resume task %s?", pretty.Sprint(cliui.DefaultStyles.Code, display)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			resp, err := client.ResumeTask(ctx, task.OwnerName, task.ID)
			if err != nil {
				return xerrors.Errorf("resume task %q: %w", display, err)
			} else if resp.WorkspaceBuild == nil {
				return xerrors.Errorf("resume task %q: no workspace build returned", display)
			}

			if noWait {
				_, _ = fmt.Fprintf(inv.Stdout, "Resuming task %q in the background.\n", cliui.Keyword(display))
				return nil
			}

			if err = cliui.WorkspaceBuild(ctx, inv.Stdout, client, resp.WorkspaceBuild.ID); err != nil {
				return xerrors.Errorf("watch resume build for task %q: %w", display, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nThe %s task has been resumed.\n", cliui.Keyword(display))
			return nil
		},
	}
	return cmd
}
