package cli

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskStatus() *serpent.Command {
	var (
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]taskListRow{},
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
					rows, ok := data.([]taskListRow)
					if !ok {
						return nil, xerrors.Errorf("expected []taskListRow, got %T", data)
					}
					if len(rows) != 1 {
						return nil, xerrors.Errorf("expected exactly 1 row, got %d", len(rows))
					}
					return rows[0].Task, nil
				},
			),
		)
		watchArg         bool
		watchIntervalArg time.Duration
	)
	cmd := &serpent.Command{
		Short: "Show the status of a task.",
		Long: FormatExamples(
			Example{
				Description: "Show the status of a given task.",
				Command:     "coder exp task status task1",
			},
			Example{
				Description: "Watch the status of a given task until it completes (idle or stopped).",
				Command:     "coder exp task status task1 --watch",
			},
		),
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
			exp := codersdk.NewExperimentalClient(client)
			identifier := i.Args[0]
			now := time.Now()

			task, err := exp.TaskByIdentifier(ctx, identifier)
			if err != nil {
				return err
			}

			tsr := taskListRowFromTask(now, task)
			out, err := formatter.Format(ctx, []taskListRow{tsr})
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
			lastRow := tsr
			for range t.C {
				task, err := exp.TaskByID(ctx, task.ID)
				if err != nil {
					return err
				}

				// Only print if something changed
				newRow := taskListRowFromTask(now, task)
				if !taskListRowEqual(lastRow, newRow) {
					out, err := formatter.Format(ctx, []taskListRow{newRow})
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

				lastRow = newRow
			}
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func taskWatchIsEnded(task codersdk.Task) bool {
	if task.WorkspaceStatus == codersdk.WorkspaceStatusStopped {
		return true
	}
	if task.WorkspaceAgentHealth == nil || !task.WorkspaceAgentHealth.Healthy {
		return false
	}
	if task.WorkspaceAgentLifecycle == nil || task.WorkspaceAgentLifecycle.Starting() || task.WorkspaceAgentLifecycle.ShuttingDown() {
		return false
	}
	if (task.AppStatus == nil || task.AppStatus.State == codersdk.WorkspaceAppStatusStateWorking) || (task.CurrentState != nil || task.CurrentState.State == codersdk.TaskStateWorking) {
		return false
	}
	return true
}

func taskListRowEqual(r1, r2 taskListRow) bool {
	return r1.Task.Status == r2.Task.Status &&
		r1.Healthy == r2.Healthy &&
		(taskStateEqual(r1.Task.CurrentState, r2.Task.CurrentState) || workspaceAppStatusEqual(r1.Task.AppStatus, r2.Task.AppStatus))
}

// TODO(cian): remove when codersdk.TaskStatus goes away
func taskStateEqual(se1, se2 *codersdk.TaskStateEntry) bool {
	var s1, m1, s2, m2 string
	if se1 != nil {
		s1 = string(se1.State)
		m1 = se1.Message
	}
	if se2 != nil {
		s2 = string(se2.State)
		m2 = se2.Message
	}
	return s1 == s2 && m1 == m2
}

func workspaceAppStatusEqual(wa1, wa2 *codersdk.WorkspaceAppStatus) bool {
	var s1, m1, s2, m2 string
	if wa1 != nil {
		s1 = string(wa1.State)
		m1 = wa1.Message
	}
	if wa2 != nil {
		s2 = string(wa2.State)
		m2 = wa2.Message
	}
	return s1 == s2 && m1 == m2
}
