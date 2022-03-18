package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/database"
)

func workspaceDelete() *cobra.Command {
	return &cobra.Command{
		Use:               "delete <workspace>",
		Aliases:           []string{"rm"},
		ValidArgsFunction: validArgsWorkspaceName,
		Args:              cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := client.WorkspaceByName(cmd.Context(), "", args[0])
			if err != nil {
				return err
			}
			before := time.Now()
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: database.WorkspaceTransitionDelete,
			})
			if err != nil {
				return err
			}
			_, err = cliui.Job(cmd, cliui.JobOptions{
				Title: "Deleting workspace...",
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
