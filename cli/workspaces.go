package cli

import (
	"strings"

	"github.com/spf13/cobra"
)

func workspaces() *cobra.Command {
	cmd := &cobra.Command{
		Use: "workspaces",
	}
	cmd.AddCommand(workspaceAgent())
	cmd.AddCommand(workspaceCreate())
	cmd.AddCommand(workspaceDelete())
	cmd.AddCommand(workspaceList())
	cmd.AddCommand(workspaceShow())
	cmd.AddCommand(workspaceStop())
	cmd.AddCommand(workspaceStart())
	cmd.AddCommand(workspaceSSH())
	cmd.AddCommand(workspaceUpdate())

	return cmd
}

func validArgsWorkspaceName(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	client, err := createClient(cmd)
	if err != nil {
		return nil, cobra.ShellCompDirectiveError
	}
	workspaces, err := client.WorkspacesByUser(cmd.Context(), "")
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
