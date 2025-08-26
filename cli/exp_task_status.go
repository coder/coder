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
		client    = new(codersdk.Client)
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]taskStatusRow{},
				[]string{
					"state changed",
					"status",
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
			r.InitClient(client),
		),
		Handler: func(i *serpent.Invocation) error {
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

			out, err := formatter.Format(ctx, toStatusRow(task))
			if err != nil {
				return xerrors.Errorf("format task status: %w", err)
			}
			_, _ = fmt.Fprintln(i.Stdout, out)

			if !watchArg {
				return nil
			}

			lastStatus := task.Status
			lastState := task.CurrentState
			t := time.NewTicker(watchIntervalArg)
			defer t.Stop()
			// TODO: implement streaming updates instead of polling
			for range t.C {
				task, err := ec.TaskByID(ctx, taskID)
				if err != nil {
					return err
				}
				if lastStatus == task.Status && taskStatusEqual(lastState, task.CurrentState) {
					continue
				}
				out, err := formatter.Format(ctx, toStatusRow(task))
				if err != nil {
					return xerrors.Errorf("format task status: %w", err)
				}
				// hack: skip the extra column header from formatter
				if formatter.FormatID() != cliui.JSONFormat().ID() {
					out = strings.SplitN(out, "\n", 2)[1]
				}
				_, _ = fmt.Fprintln(i.Stdout, out)

				if task.Status == codersdk.WorkspaceStatusStopped {
					return nil
				}
				lastStatus = task.Status
				lastState = task.CurrentState
			}
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func taskStatusEqual(s1, s2 *codersdk.TaskStateEntry) bool {
	if s1 == nil && s2 == nil {
		return true
	}
	if s1 == nil || s2 == nil {
		return false
	}
	return s1.State == s2.State
}

type taskStatusRow struct {
	codersdk.Task `table:"-"`
	ChangedAgo    string    `json:"-" table:"state changed,default_sort"`
	Timestamp     time.Time `json:"-" table:"-"`
	TaskStatus    string    `json:"-" table:"status"`
	TaskState     string    `json:"-" table:"state"`
	Message       string    `json:"-" table:"message"`
}

func toStatusRow(task codersdk.Task) []taskStatusRow {
	tsr := taskStatusRow{
		Task:       task,
		ChangedAgo: time.Since(task.UpdatedAt).Truncate(time.Second).String() + " ago",
		Timestamp:  task.UpdatedAt,
		TaskStatus: string(task.Status),
	}
	if task.CurrentState != nil {
		tsr.ChangedAgo = time.Since(task.CurrentState.Timestamp).Truncate(time.Second).String() + " ago"
		tsr.Timestamp = task.CurrentState.Timestamp
		tsr.TaskState = string(task.CurrentState.State)
		tsr.Message = task.CurrentState.Message
	}
	return []taskStatusRow{tsr}
}
