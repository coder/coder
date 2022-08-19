package cli

import (
	"fmt"
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
		if !workspace.LatestBuild.Deadline.IsZero() && workspace.LatestBuild.Deadline.After(now) && status == "Running" {
			remaining := time.Until(workspace.LatestBuild.Deadline)
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
	var columns []string
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List all workspaces",
		Aliases:     []string{"ls"},
		Args:        cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			workspaces, err := client.Workspaces(cmd.Context(), codersdk.WorkspaceFilter{})
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
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
			displayWorkspaces := make([]workspaceListRow, len(workspaces))
			for i, workspace := range workspaces {
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
	cmd.Flags().StringArrayVarP(&columns, "column", "c", nil,
		"Specify a column to filter in the table.")
	return cmd
}
