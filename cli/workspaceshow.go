package cli

import (
	"github.com/spf13/cobra"
)

func workspaceShow() *cobra.Command {
	return &cobra.Command{
		Use: "show",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
