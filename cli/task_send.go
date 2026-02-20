package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskSend() *serpent.Command {
	var stdin bool

	cmd := &serpent.Command{
		Use:   "send <task> [<input> | --stdin]",
		Short: "Send input to a task",
		Long: "Send input to a task. If the task is paused, it will be automatically resumed before input is sent. If the task is initializing, it will wait for the task to become ready.\n" +
			FormatExamples(Example{
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

			task, err := client.TaskByIdentifier(ctx, identifier)
			if err != nil {
				return xerrors.Errorf("resolve task: %w", err)
			}

			display := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)

			// Before attempting to send, check the task status and
			// handle non-active states.
			switch task.Status {
			case codersdk.TaskStatusActive:
				// Ready to send, fall through.

			case codersdk.TaskStatusPaused:
				resp, err := client.ResumeTask(ctx, task.OwnerName, task.ID)
				if err != nil {
					return xerrors.Errorf("resume task %q: %w", display, err)
				} else if resp.WorkspaceBuild == nil {
					return xerrors.Errorf("resume task %q", display)
				}

				if err = waitForTaskReady(ctx, inv, client, task, resp.WorkspaceBuild.ID); err != nil {
					return err
				}

			case codersdk.TaskStatusInitializing:
				if !task.WorkspaceID.Valid {
					return xerrors.Errorf("send input to task %q: task has no backing workspace", display)
				}

				workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
				if err != nil {
					return xerrors.Errorf("get workspace for task %q: %w", display, err)
				}

				if err = waitForTaskReady(ctx, inv, client, task, workspace.LatestBuild.ID); err != nil {
					return err
				}

			default:
				return xerrors.Errorf("task %q has status %s and cannot be sent input", display, task.Status)
			}

			if err := client.TaskSend(ctx, codersdk.Me, task.ID, codersdk.TaskSendRequest{Input: taskInput}); err != nil {
				return xerrors.Errorf("send input to task %q: %w", display, err)
			}

			return nil
		},
	}

	return cmd
}

// waitForTaskReady watches the workspace build to completion then polls
// until the task becomes active.
func waitForTaskReady(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, task codersdk.Task, workspaceBuildID uuid.UUID) error {
	display := fmt.Sprintf("%s/%s", task.OwnerName, task.Name)

	if err := cliui.WorkspaceBuild(ctx, inv.Stdout, client, workspaceBuildID); err != nil {
		return xerrors.Errorf("watch workspace build for task %q: %w", display, err)
	}

	// TODO(DanielleMaywood):
	// When we have a streaming Task API, this should be converted away from polling.

	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			task, err := client.TaskByID(ctx, task.ID)
			if err != nil {
				return xerrors.Errorf("get task by id: %w", err)
			} else if task.Status == codersdk.TaskStatusError || task.Status == codersdk.TaskStatusUnknown {
				return xerrors.Errorf("entered %s state while waiting for it to become active", task.Status)
			}

			if task.Status == codersdk.TaskStatusActive {
				return nil
			}
		}
	}
}
