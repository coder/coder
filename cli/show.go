package cli

import (
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func show() *cobra.Command {
	return &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "show",
		Short:       "Show details of a workspace's resources and agents",
		Args:        cobra.ExactArgs(1),
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
				return xerrors.Errorf("get workspace: %w", err)
			}
			resources, err := client.WorkspaceResourcesByBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return xerrors.Errorf("get workspace resources: %w", err)
			}
			return cliui.WorkspaceResources(cmd.OutOrStdout(), resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspace.Name,
			})
		},
	}
}
