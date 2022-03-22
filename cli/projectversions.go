package cli

import "github.com/spf13/cobra"

func projectVersions() *cobra.Command {
	return &cobra.Command{
		Use:     "versions",
		Aliases: []string{"version"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}

// coder project versions
