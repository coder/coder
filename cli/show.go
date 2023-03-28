package cli

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) show() *clibase.Cmd {
	client := new(codersdk.Client)
	return &clibase.Cmd{
		Use:   "show <workspace>",
		Short: "Display details of a workspace's resources and agents",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			buildInfo, err := client.BuildInfo(inv.Context())
			if err != nil {
				return xerrors.Errorf("get server version: %w", err)
			}
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}
			return cliui.WorkspaceResources(inv.Stdout, workspace.LatestBuild.Resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspace.Name,
				ServerVersion: buildInfo.Version,
			})
		},
	}
}
