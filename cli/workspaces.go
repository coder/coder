package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/coder/coder/codersdk"
)

func workspaces() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "workspaces",
		Aliases: []string{"ws"},
	}
	cmd.AddCommand(workspaceCreate())
	cmd.AddCommand(workspaceDelete())
	cmd.AddCommand(workspaceList())
	cmd.AddCommand(workspaceShow())
	cmd.AddCommand(workspaceStop())
	cmd.AddCommand(workspaceStart())
	cmd.AddCommand(ssh())
	cmd.AddCommand(workspaceUpdate())
	cmd.AddCommand(workspaceAutostart())
	cmd.AddCommand(workspaceAutostop())

	return cmd
}

func validArgsWorkspaceName(cmd *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	client, err := createClient(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	organization, err := currentOrganization(cmd, client)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	workspaces, err := client.WorkspacesByOwner(cmd.Context(), organization.ID, codersdk.Me)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	names := make([]string, 0)
	for _, workspace := range workspaces {
		if !strings.HasPrefix(workspace.Name, toComplete) {
			continue
		}
		names = append(names, workspace.Name)
	}
	return names, cobra.ShellCompDirectiveDefault
}
