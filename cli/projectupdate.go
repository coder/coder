package cli

import "github.com/spf13/cobra"

func projectUpdate() *cobra.Command {
	return &cobra.Command{
		Use:   "update <name>",
		Args:  cobra.MinimumNArgs(1),
		Short: "Update a project from the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
