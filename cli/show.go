package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func show(root *RootCmd) *clibase.Command {
	var client *codersdk.Client
	return &clibase.Command{
		Use:   "show <workspace>",
		Short: "Display details of a workspace's resources and agents",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			root.useClient(client),
		),
		Handler: func(i *clibase.Invokation) error {
			buildInfo, err := client.BuildInfo(i.Context())
			if err != nil {
				return xerrors.Errorf("get server version: %w", err)
			}
			workspace, err := namedWorkspace(i.Context(), client, i.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}
			return cliui.WorkspaceResources(i.Stdout, workspace.LatestBuild.Resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspace.Name,
				ServerVersion: buildInfo.Version,
			})
		},
	}
}
