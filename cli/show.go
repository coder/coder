package cli

import (
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
)

func show() *cobra.Command {
	return &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "show <workspace>",
		Short:       "Display details of a workspace's resources and agents",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			buildInfo, err := client.BuildInfo(cmd.Context())
			if err != nil {
				return xerrors.Errorf("get server version: %w", err)
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}
			return cliui.WorkspaceResources(cmd.OutOrStdout(), workspace.LatestBuild.Resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspace.Name,
				ServerVersion: buildInfo.Version,
			})
		},
	}
}
