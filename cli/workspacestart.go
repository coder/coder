package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func workspaceStart() *cobra.Command {
	return &cobra.Command{
		Use:               "start <workspace>",
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
			if err != nil {
				return err
			}
			before := time.Now()
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: database.WorkspaceTransitionStart,
			})
			if err != nil {
				return err
			}
			err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					build, err := client.WorkspaceBuild(cmd.Context(), build.ID)
					return build.Job, err
				},
				Cancel: func() error {
					return client.CancelWorkspaceBuild(cmd.Context(), build.ID)
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
					return client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, before)
				},
			})
			return err
		},
	}
}
