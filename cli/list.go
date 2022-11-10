package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

type workspaceListRow struct {
	Workspace  string `table:"workspace"`
	Template   string `table:"template"`
	Status     string `table:"status"`
	LastBuilt  string `table:"last built"`
	Outdated   bool   `table:"outdated"`
	StartsAt   string `table:"starts at"`
	StopsAfter string `table:"stops after"`
}

func workspaceListRowFromWorkspace(now time.Time, usersByID map[uuid.UUID]codersdk.User, workspace codersdk.Workspace) workspaceListRow {
	status := codersdk.WorkspaceDisplayStatus(workspace.LatestBuild.Job.Status, workspace.LatestBuild.Transition)

	lastBuilt := now.UTC().Sub(workspace.LatestBuild.Job.CreatedAt).Truncate(time.Second)
	autostartDisplay := "-"
	if !ptr.NilOrEmpty(workspace.AutostartSchedule) {
		if sched, err := schedule.Weekly(*workspace.AutostartSchedule); err == nil {
			autostartDisplay = fmt.Sprintf("%s %s (%s)", sched.Time(), sched.DaysOfWeek(), sched.Location())
		}
	}

	autostopDisplay := "-"
	if !ptr.NilOrZero(workspace.TTLMillis) {
		dur := time.Duration(*workspace.TTLMillis) * time.Millisecond
		autostopDisplay = durationDisplay(dur)
		if !workspace.LatestBuild.Deadline.IsZero() && workspace.LatestBuild.Deadline.Time.After(now) && status == "Running" {
			remaining := time.Until(workspace.LatestBuild.Deadline.Time)
			autostopDisplay = fmt.Sprintf("%s (%s)", autostopDisplay, relative(remaining))
		}
	}

	user := usersByID[workspace.OwnerID]
	return workspaceListRow{
		Workspace:  user.Username + "/" + workspace.Name,
		Template:   workspace.TemplateName,
		Status:     status,
		LastBuilt:  durationDisplay(lastBuilt),
		Outdated:   workspace.Outdated,
		StartsAt:   autostartDisplay,
		StopsAfter: autostopDisplay,
	}
}

func list() *cobra.Command {
	var (
		all               bool
		columns           []string
		defaultQuery      = "owner:me"
		searchQuery       string
		me                bool
		displayWorkspaces []workspaceListRow
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List workspaces",
		Aliases:     []string{"ls"},
		Args:        cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			filter := codersdk.WorkspaceFilter{
				FilterQuery: searchQuery,
			}
			if all && searchQuery == defaultQuery {
				filter.FilterQuery = ""
			}

			if me {
				myUser, err := client.User(cmd.Context(), codersdk.Me)
				if err != nil {
					return err
				}
				filter.Owner = myUser.Username
			}
			res, err := client.Workspaces(cmd.Context(), filter)
			if err != nil {
				return err
			}
			if len(res.Workspaces) == 0 {
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Prompt.String()+"No workspaces found! Create one:")
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "  "+cliui.Styles.Code.Render("coder create <name>"))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr())
				return nil
			}
			users, err := client.Users(cmd.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}
			usersByID := map[uuid.UUID]codersdk.User{}
			for _, user := range users {
				usersByID[user.ID] = user
			}

			now := time.Now()
			displayWorkspaces = make([]workspaceListRow, len(res.Workspaces))
			for i, workspace := range res.Workspaces {
				displayWorkspaces[i] = workspaceListRowFromWorkspace(now, usersByID, workspace)
			}

			out, err := cliui.DisplayTable(displayWorkspaces, "workspace", columns)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	availColumns, err := cliui.TableHeaders(displayWorkspaces)
	if err != nil {
		panic(err)
	}
	columnString := strings.Join(availColumns[:], ", ")

	cmd.Flags().BoolVarP(&all, "all", "a", false,
		"Specifies whether all workspaces will be listed or not.")
	cmd.Flags().StringArrayVarP(&columns, "column", "c", nil,
		fmt.Sprintf("Specify a column to filter in the table. Available columns are: %v", columnString))
	cmd.Flags().StringVar(&searchQuery, "search", "", "Search for a workspace with a query.")
	return cmd
}
