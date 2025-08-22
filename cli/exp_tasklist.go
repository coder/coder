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

type taskListRow struct {
	Task codersdk.Task `table:"t,recursive_inline"`

	StateChanged string `table:"state changed"`
}

func taskListRowFromTask(now time.Time, t codersdk.Task) taskListRow {
	var stateAgo string
	if t.CurrentState != nil {
		stateAgo = now.UTC().Sub(t.CurrentState.Timestamp).Truncate(time.Second).String() + " ago"
	}

	return taskListRow{
		Task: t,

		StateChanged: stateAgo,
	}
}

func (r *RootCmd) taskList() *serpent.Command {
	var (
		statusFilter string
		all          bool
		user         string

		client    = new(codersdk.Client)
		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]taskListRow{},
				[]string{
					"id",
					"name",
					"status",
					"state",
					"state changed",
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
					out := make([]codersdk.Task, len(rows))
					for i := range rows {
						out[i] = rows[i].Task
					}
					return out, nil
				},
			),
		)
	)

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List experimental tasks",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			{
				Name:        "status",
				Description: "Filter by task status (e.g. running, failed, etc).",
				Flag:        "status",
				Default:     "",
				Value:       serpent.StringOf(&statusFilter),
			},
			{
				Name:          "all",
				Description:   "List tasks for all users you can view.",
				Flag:          "all",
				FlagShorthand: "a",
				Default:       "false",
				Value:         serpent.BoolOf(&all),
			},
			{
				Name:        "user",
				Description: "List tasks for the specified user (username, \"me\").",
				Flag:        "user",
				Default:     "",
				Value:       serpent.StringOf(&user),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			exp := codersdk.NewExperimentalClient(client)

			targetUser := strings.TrimSpace(user)
			if targetUser == "" && !all {
				targetUser = codersdk.Me
			}

			tasks, err := exp.Tasks(ctx, &codersdk.TasksFilter{
				Owner:  targetUser,
				Status: statusFilter,
			})
			if err != nil {
				return xerrors.Errorf("list tasks: %w", err)
			}

			// If no rows and not JSON, show a friendly message.
			if len(tasks) == 0 && formatter.FormatID() != cliui.JSONFormat().ID() {
				_, _ = fmt.Fprintln(inv.Stderr, "No tasks found.")
				return nil
			}

			rows := make([]taskListRow, len(tasks))
			now := time.Now()
			for i := range tasks {
				rows[i] = taskListRowFromTask(now, tasks[i])
			}

			out, err := formatter.Format(ctx, rows)
			if err != nil {
				return xerrors.Errorf("format tasks: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
