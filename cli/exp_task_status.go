package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskStatus() *serpent.Command {
	var (
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]taskStatusRow{},
				[]string{
					"state changed",
					"status",
					"healthy",
					"state",
					"message",
				},
			),
			cliui.ChangeFormatterData(
				cliui.JSONFormat(),
				func(data any) (any, error) {
					rows, ok := data.([]taskStatusRow)
					if !ok {
						return nil, xerrors.Errorf("expected []taskStatusRow, got %T", data)
					}
					if len(rows) != 1 {
						return nil, xerrors.Errorf("expected exactly 1 row, got %d", len(rows))
					}
					return rows[0], nil
				},
			),
		)
		watchArg         bool
		watchIntervalArg time.Duration
	)
	cmd := &serpent.Command{
		Short:   "Show the status of a task.",
		Use:     "status",
		Aliases: []string{"stat"},
		Options: serpent.OptionSet{
			{
				Default:     "false",
				Description: "Watch the task status output. This will stream updates to the terminal until the underlying workspace is stopped.",
				Flag:        "watch",
				Name:        "watch",
				Value:       serpent.BoolOf(&watchArg),
			},
			{
				Default:     "1s",
				Description: "Interval to poll the task for updates. Only used in tests.",
				Hidden:      true,
				Flag:        "watch-interval",
				Name:        "watch-interval",
				Value:       serpent.DurationOf(&watchIntervalArg),
			},
		},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(i *serpent.Invocation) error {
			client, err := r.InitClient(i)
			if err != nil {
				return err
			}

			ctx := i.Context()
			ec := codersdk.NewExperimentalClient(client)
			identifier := i.Args[0]

			taskID, err := uuid.Parse(identifier)
			if err != nil {
				// Try to resolve the task as a named workspace
				// TODO: right now tasks are still "workspaces" under the hood.
				// We should update this once we have a proper task model.
				ws, err := namedWorkspace(ctx, client, identifier)
				if err != nil {
					return err
				}
				taskID = ws.ID
			}
			task, err := ec.TaskByID(ctx, taskID)
			if err != nil {
				return err
			}

			tsr := toStatusRow(task)
			out, err := formatter.Format(ctx, []taskStatusRow{tsr})
			if err != nil {
				return xerrors.Errorf("format task status: %w", err)
			}
			_, _ = fmt.Fprintln(i.Stdout, out)

			if !watchArg || taskWatchIsEnded(task) {
				return nil
			}

			t := time.NewTicker(watchIntervalArg)
			defer t.Stop()
			// TODO: implement streaming updates instead of polling
			lastStatusRow := tsr
			for range t.C {
				task, err := ec.TaskByID(ctx, taskID)
				if err != nil {
					return err
				}

				// Only print if something changed
				newStatusRow := toStatusRow(task)
				if !taskStatusRowEqual(lastStatusRow, newStatusRow) {
					out, err := formatter.Format(ctx, []taskStatusRow{newStatusRow})
					if err != nil {
						return xerrors.Errorf("format task status: %w", err)
					}
					// hack: skip the extra column header from formatter
					if formatter.FormatID() != cliui.JSONFormat().ID() {
						out = strings.SplitN(out, "\n", 2)[1]
					}
					_, _ = fmt.Fprintln(i.Stdout, out)
				}

				if taskWatchIsEnded(task) {
					return nil
				}

				lastStatusRow = newStatusRow
			}
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func taskWatchIsEnded(task codersdk.Task) bool {
	if task.Status == codersdk.WorkspaceStatusStopped {
		return true
	}
	if task.WorkspaceAgentHealth == nil || !task.WorkspaceAgentHealth.Healthy {
		return false
	}
	if task.WorkspaceAgentLifecycle == nil || task.WorkspaceAgentLifecycle.Starting() || task.WorkspaceAgentLifecycle.ShuttingDown() {
		return false
	}
	if task.CurrentState == nil || task.CurrentState.State == codersdk.TaskStateWorking {
		return false
	}
	return true
}

type taskStatusRow struct {
	codersdk.Task `table:"-"`
	ChangedAgo    string    `json:"-" table:"state changed,default_sort"`
	Timestamp     time.Time `json:"-" table:"-"`
	TaskStatus    string    `json:"-" table:"status"`
	Healthy       bool      `json:"-" table:"healthy"`
	TaskState     string    `json:"-" table:"state"`
	Message       string    `json:"-" table:"message"`
}

func taskStatusRowEqual(r1, r2 taskStatusRow) bool {
	return r1.TaskStatus == r2.TaskStatus &&
		r1.Healthy == r2.Healthy &&
		r1.TaskState == r2.TaskState &&
		r1.Message == r2.Message
}

func toStatusRow(task codersdk.Task) taskStatusRow {
	tsr := taskStatusRow{
		Task:       task,
		ChangedAgo: time.Since(task.UpdatedAt).Truncate(time.Second).String() + " ago",
		Timestamp:  task.UpdatedAt,
		TaskStatus: string(task.Status),
	}
	tsr.Healthy = task.WorkspaceAgentHealth != nil &&
		task.WorkspaceAgentHealth.Healthy &&
		task.WorkspaceAgentLifecycle != nil &&
		!task.WorkspaceAgentLifecycle.Starting() &&
		!task.WorkspaceAgentLifecycle.ShuttingDown()

	if task.CurrentState != nil {
		tsr.ChangedAgo = time.Since(task.CurrentState.Timestamp).Truncate(time.Second).String() + " ago"
		tsr.Timestamp = task.CurrentState.Timestamp
		tsr.TaskState = string(task.CurrentState.State)
		tsr.Message = task.CurrentState.Message
	}
	return tsr
}
