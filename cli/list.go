package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func list() *cobra.Command {
	var columns []string
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List all workspaces",
		Aliases:     []string{"ls"},
		Args:        cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspaces, err := client.Workspaces(cmd.Context(), codersdk.WorkspaceFilter{})
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"No workspaces found! Create one:")
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render("coder create <name>"))
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
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

			tableWriter := cliui.Table()
			header := table.Row{"workspace", "template", "status", "last built", "outdated", "starts at", "stops after"}
			tableWriter.AppendHeader(header)
			tableWriter.SortBy([]table.SortBy{{
				Name: "workspace",
			}})
			tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, columns))

			now := time.Now()
			for _, workspace := range workspaces {
				status := codersdk.WorkspaceDisplayStatus(workspace.LatestBuild.Job.Status, workspace.LatestBuild.Transition)

				lastBuilt := time.Now().UTC().Sub(workspace.LatestBuild.Job.CreatedAt).Truncate(time.Second)
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
				tableWriter.AppendRow(table.Row{
					user.Username + "/" + workspace.Name,
					workspace.TemplateName,
					status,
					durationDisplay(lastBuilt),
					workspace.Outdated,
					autostartDisplay,
					autostopDisplay,
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", nil,
		"Specify a column to filter in the table.")
	return cmd
}
