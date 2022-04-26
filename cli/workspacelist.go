package cli

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func workspaceList() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			workspaces, err := client.WorkspacesByOwner(cmd.Context(), organization.ID, codersdk.Me)
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"No workspaces found! Create one:")
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render("coder workspaces create <name>"))
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				return nil
			}

			tableWriter := table.NewWriter()
			tableWriter.SetStyle(table.StyleLight)
			tableWriter.Style().Options.SeparateColumns = false
			tableWriter.AppendHeader(table.Row{"Workspace", "Template", "Status", "Last Built", "Outdated"})

			for _, workspace := range workspaces {
				status := ""
				inProgress := false
				if workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobRunning ||
					workspace.LatestBuild.Job.Status == codersdk.ProvisionerJobCanceling {
					inProgress = true
				}

				switch workspace.LatestBuild.Transition {
				case database.WorkspaceTransitionStart:
					status = "start"
					if inProgress {
						status = "starting"
					}
				case database.WorkspaceTransitionStop:
					status = "stop"
					if inProgress {
						status = "stopping"
					}
				case database.WorkspaceTransitionDelete:
					status = "delete"
					if inProgress {
						status = "deleting"
					}
				}

				tableWriter.AppendRow(table.Row{
					cliui.Styles.Bold.Render(workspace.Name),
					workspace.TemplateName,
					status,
					workspace.LatestBuild.Job.CreatedAt.Format("January 2, 2006"),
					workspace.Outdated,
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
}
