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
		Long: `Send input to a task. If the task is paused, it will be automatically resumed before input is sent. If the task is initializing, it will wait for the task to become ready.
` +
			FormatExamples(Example{
				Description: "Send direct input to a task",
				Command:     `coder task send task1 "Please also add unit tests"`,
			}, Example{
				Description: "Send input from stdin to a task",
				Command:     `echo "Please also add unit tests" | coder task send task1 --stdin`,
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
			var workspaceBuildID uuid.UUID

			switch task.Status {
			case codersdk.TaskStatusActive:
				// Already active, no build to watch.

			case codersdk.TaskStatusPaused:
				resp, err := client.ResumeTask(ctx, task.OwnerName, task.ID)
				if err != nil {
					return xerrors.Errorf("resume task %q: %w", display, err)
				} else if resp.WorkspaceBuild == nil {
					return xerrors.Errorf("resume task %q", display)
				}

				workspaceBuildID = resp.WorkspaceBuild.ID

			case codersdk.TaskStatusInitializing:
				if !task.WorkspaceID.Valid {
					return xerrors.Errorf("send input to task %q: task has no backing workspace", display)
				}

				workspace, err := client.Workspace(ctx, task.WorkspaceID.UUID)
				if err != nil {
					return xerrors.Errorf("get workspace for task %q: %w", display, err)
				}

				workspaceBuildID = workspace.LatestBuild.ID

			default:
				return xerrors.Errorf("task %q has status %s and cannot be sent input", display, task.Status)
			}

			if err := waitForTaskIdle(ctx, inv, client, task, workspaceBuildID); err != nil {
				return xerrors.Errorf("wait for task %q to be idle: %w", display, err)
			}

			if err := client.TaskSend(ctx, codersdk.Me, task.ID, codersdk.TaskSendRequest{Input: taskInput}); err != nil {
				return xerrors.Errorf("send input to task %q: %w", display, err)
			}

			return nil
		},
	}

	return cmd
}

// waitForTaskIdle optionally watches a workspace build to completion,
// then polls until the task becomes active and its app state is idle.
// This merges build-watching and idle-polling into a single loop so
// that status changes (e.g. paused) are never missed between phases.
func waitForTaskIdle(ctx context.Context, inv *serpent.Invocation, client *codersdk.Client, task codersdk.Task, workspaceBuildID uuid.UUID) error {
	if workspaceBuildID != uuid.Nil {
		if err := cliui.WorkspaceBuild(ctx, inv.Stdout, client, workspaceBuildID); err != nil {
			return xerrors.Errorf("watch workspace build: %w", err)
		}
	}

	cliui.Infof(inv.Stdout, "Waiting for task to become idle...")

	// NOTE(DanielleMaywood):
	// It has been observed that the `TaskStatusError` state has
	// appeared during a typical healthy startup [^0]. To combat
	// this, we allow a 5 minute grace period where we allow
	// `TaskStatusError` to surface without immediately failing.
	//
	// TODO(DanielleMaywood):
	// Remove this grace period once the upstream agentapi health
	// check no longer reports transient error states during normal
	// startup.
	//
	// [0]: https://github.com/coder/coder/pull/22203#discussion_r2858002569
	const errorGracePeriod = 5 * time.Minute
	gracePeriodDeadline := time.Now().Add(errorGracePeriod)

	// NOTE(DanielleMaywood):
	// On resume the MCP may not report an initial app status,
	// leaving CurrentState nil indefinitely. To avoid hanging
	// forever we treat Active with nil CurrentState as idle
	// after a grace period, giving the MCP time to report
	// during normal startup.
	const nilStateGracePeriod = 30 * time.Second
	var nilStateDeadline time.Time

	// TODO(DanielleMaywood):
	// When we have a streaming Task API, this should be converted
	// away from polling.
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			task, err := client.TaskByID(ctx, task.ID)
			if err != nil {
				return xerrors.Errorf("get task by id: %w", err)
			}

			switch task.Status {
			case codersdk.TaskStatusInitializing,
				codersdk.TaskStatusPending:
				// Not yet active, keep polling.
				continue
			case codersdk.TaskStatusActive:
				// Task is active; check app state.
				if task.CurrentState == nil {
					// The MCP may not have reported state yet.
					// Start a grace period on first observation
					// and treat as idle once it expires.
					if nilStateDeadline.IsZero() {
						nilStateDeadline = time.Now().Add(nilStateGracePeriod)
					}
					if time.Now().After(nilStateDeadline) {
						return nil
					}
					continue
				}
				// Reset nil-state deadline since we got a real
				// state report.
				nilStateDeadline = time.Time{}
				switch task.CurrentState.State {
				case codersdk.TaskStateIdle,
					codersdk.TaskStateComplete,
					codersdk.TaskStateFailed:
					return nil
				default:
					// Still working, keep polling.
					continue
				}
			case codersdk.TaskStatusError:
				if time.Now().After(gracePeriodDeadline) {
					return xerrors.Errorf("task entered %s state while waiting for it to become idle", task.Status)
				}
			case codersdk.TaskStatusPaused:
				return xerrors.Errorf("task was paused while waiting for it to become idle")
			case codersdk.TaskStatusUnknown:
				return xerrors.Errorf("task entered %s state while waiting for it to become idle", task.Status)
			default:
				return xerrors.Errorf("task entered unexpected state (%s) while waiting for it to become idle", task.Status)
			}
		}
	}
}
