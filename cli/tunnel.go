package cli

import "github.com/spf13/cobra"

func tunnel() *cobra.Command {
	return &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "tunnel",
		Short:       "Forward ports to your local machine",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
