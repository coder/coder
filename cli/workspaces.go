package cli

import "github.com/spf13/cobra"

func workspaces() *cobra.Command {
	cmd := &cobra.Command{
		Use: "workspaces",
	}
	cmd.AddCommand(workspaceAgent())
	cmd.AddCommand(workspaceCreate())

	return cmd
}
