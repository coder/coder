package cli

import (
	"github.com/spf13/cobra"
)

func projectPlan() *cobra.Command {
	return &cobra.Command{
		Use:   "plan <directory>",
		Args:  cobra.MinimumNArgs(1),
		Short: "Plan a project update from the current directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
