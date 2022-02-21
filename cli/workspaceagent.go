package cli

import (
	"github.com/spf13/cobra"
)

func workspaceAgent() *cobra.Command {
	return &cobra.Command{
		Use:    "agent",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
