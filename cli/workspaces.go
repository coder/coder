package cli

import "github.com/spf13/cobra"

func workspaces() *cobra.Command {
	cmd := &cobra.Command{
		Use: "workspaces",
	}

	return cmd
}
