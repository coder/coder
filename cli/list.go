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
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "list",
		Short:       "List all workspaces",
		Aliases:     []string{"ls"},
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

			for _, workspace := range workspaces {
				status := ""
				inProgress := false
				if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobRunning ||
					workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobCanceling {
					inProgress = true
				}

				switch workspace.LatestBuild.Transition {
				case codersdk.WorkspaceTransitionStart:
					status = "Running"
					if inProgress {
						status = "Starting"
					}
				case codersdk.WorkspaceTransitionStop:
					status = "Stopped"
					if inProgress {
						status = "Stopping"
					}
				case codersdk.WorkspaceTransitionDelete:
					status = "Deleted"
					if inProgress {
						status = "Deleting"
					}
				}
				if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobFailed {
					status = "Failed"
				}

				duration := time.Now().UTC().Sub(workspace.LatestBuild.Job.CreatedAt).Truncate(time.Second)
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
					if has, ext := hasExtension(workspace); has {
						autostopDisplay += fmt.Sprintf(" (%s%s)", sign(ext), durationDisplay(ext))
					}
				}

				user := usersByID[workspace.OwnerID]
				tableWriter.AppendRow(table.Row{
					user.Username + "/" + workspace.Name,
					workspace.TemplateName,
					status,
					durationDisplay(duration),
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
