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
				[]taskStatusRow{},
				[]string{
					"state changed",
					"task status",
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
		Short: "Show the status of a task.",
		Long: FormatExamples(
			Example{
				Description: "Show the status of a given task.",
				Command:     "coder task status task1",
			},
			Example{
				Description: "Watch the status of a given task until it completes (idle or stopped).",
				Command:     "coder task status task1 --watch",
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

			task, err := exp.TaskByIdentifier(ctx, identifier)
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
				task, err := exp.TaskByID(ctx, task.ID)
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
	if task.WorkspaceStatus == codersdk.WorkspaceStatusStopped {
		return true
	}
	if task.WorkspaceAgentHealth == nil || !task.WorkspaceAgentHealth.Healthy {
		return false
	}
	if task.WorkspaceAgentLifecycle == nil || task.WorkspaceAgentLifecycle.Starting() || task.WorkspaceAgentLifecycle.ShuttingDown() {
		return false
	}
	// Check if we have a state to examine
	if task.AppStatus != nil {
		// If AppStatus says we're working, keep watching
		if task.AppStatus.State == codersdk.WorkspaceAppStatusStateWorking {
			return false
		}
		// AppStatus has a terminal state, we're done
		return true
	}
	if task.CurrentState != nil {
		// If CurrentState says we're working, keep watching
		if task.CurrentState.State == codersdk.TaskStateWorking {
			return false
		}
		// CurrentState has a terminal state, we're done
		return true
	}
	// No state available yet, keep watching
	return false
}

type taskStatusRow struct {
	codersdk.Task `table:"r,recursive_inline"`
	ChangedAgo    string `json:"-" table:"state changed"`
	Healthy       bool   `json:"-" table:"healthy"`
}

func taskStatusRowEqual(r1, r2 taskStatusRow) bool {
	return r1.Status == r2.Status &&
		r1.Healthy == r2.Healthy &&
		taskStateEqual(r1.CurrentState, r2.CurrentState)
}

func toStatusRow(task codersdk.Task) taskStatusRow {
	tsr := taskStatusRow{
		Task:       task,
		ChangedAgo: time.Since(task.UpdatedAt).Truncate(time.Second).String() + " ago",
	}
	tsr.Healthy = task.WorkspaceAgentHealth != nil &&
		task.WorkspaceAgentHealth.Healthy &&
		task.WorkspaceAgentLifecycle != nil &&
		!task.WorkspaceAgentLifecycle.Starting() &&
		!task.WorkspaceAgentLifecycle.ShuttingDown()

	if task.AppStatus != nil {
		tsr.ChangedAgo = relative(time.Since(task.AppStatus.CreatedAt).Truncate(time.Second))
	} else if task.CurrentState != nil {
		tsr.ChangedAgo = relative(time.Since(task.CurrentState.Timestamp).Truncate(time.Second))
	}
	return tsr
}

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
