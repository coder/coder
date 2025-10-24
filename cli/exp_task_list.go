package cli

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

type taskListRow struct {
	Task codersdk.Task `table:"t,recursive_inline"`

	OwnerAndName    string    `table:"task,default_sort"`
	StateChangedAgo string    `table:"state changed"`
	Healthy         bool      `json:"-" table:"healthy"`
	Message         string    `json:"message" table:"message"`
	State           string    `json:"-" table:"state"`
	Timestamp       time.Time `json:"timestamp" format:"date-time" table:"-"`
	URI             string    `json:"uri" table:"-"`
}

func taskListRowFromTask(now time.Time, t codersdk.Task) taskListRow {
	tsr := taskListRow{
		Task:         t,
		OwnerAndName: fmt.Sprintf("%s/%s", t.OwnerName, t.Name),
		Healthy: t.WorkspaceAgentHealth != nil &&
			t.WorkspaceAgentHealth.Healthy &&
			t.WorkspaceAgentLifecycle != nil &&
			!t.WorkspaceAgentLifecycle.Starting() &&
			!t.WorkspaceAgentLifecycle.ShuttingDown(),
	}
	if t.AppStatus != nil {
		tsr.StateChangedAgo = relative(-now.UTC().Sub(t.AppStatus.CreatedAt).Truncate(time.Second))
		tsr.Message = t.AppStatus.Message
		tsr.State = string(t.AppStatus.State)
		tsr.Timestamp = t.AppStatus.CreatedAt
		tsr.URI = t.AppStatus.URI
	} else if t.CurrentState != nil {
		tsr.StateChangedAgo = relative(-now.UTC().Sub(t.CurrentState.Timestamp).Truncate(time.Second))
		tsr.Message = t.CurrentState.Message
		tsr.State = string(t.CurrentState.State)
		tsr.Timestamp = t.CurrentState.Timestamp
		tsr.URI = t.CurrentState.URI
	}

	return tsr
}

func (r *RootCmd) taskList() *serpent.Command {
	var (
		statusFilter string
		all          bool
		user         string
		quiet        bool

		formatter = cliui.NewOutputFormatter(
			cliui.TableFormat(
				[]taskListRow{},
				[]string{
					"task",
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
		Use:   "list",
		Short: "List experimental tasks",
		Long: FormatExamples(
			Example{
				Description: "List tasks for the current user.",
				Command:     "coder exp task list",
			},
			Example{
				Description: "List tasks for a specific user.",
				Command:     "coder exp task list --user someone-else",
			},
			Example{
				Description: "List all tasks you can view.",
				Command:     "coder exp task list --all",
			},
			Example{
				Description: "List all your running tasks.",
				Command:     "coder exp task list --status running",
			},
			Example{
				Description: "As above, but only show IDs.",
				Command:     "coder exp task list --status running --quiet",
			},
		),
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Options: serpent.OptionSet{
			{
				Name:        "status",
				Description: "Filter by task status.",
				Flag:        "status",
				Default:     "",
				Value:       serpent.EnumOf(&statusFilter, slice.ToStrings(codersdk.AllTaskStatuses())...),
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
			{
				Name:          "quiet",
				Description:   "Only display task IDs.",
				Flag:          "quiet",
				FlagShorthand: "q",
				Default:       "false",
				Value:         serpent.BoolOf(&quiet),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()
			exp := codersdk.NewExperimentalClient(client)

			targetUser := strings.TrimSpace(user)
			if targetUser == "" && !all {
				targetUser = codersdk.Me
			}

			tasks, err := exp.Tasks(ctx, &codersdk.TasksFilter{
				Owner:  targetUser,
				Status: codersdk.TaskStatus(statusFilter),
			})
			if err != nil {
				return xerrors.Errorf("list tasks: %w", err)
			}

			if quiet {
				for _, task := range tasks {
					_, _ = fmt.Fprintln(inv.Stdout, task.ID.String())
				}

				return nil
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
