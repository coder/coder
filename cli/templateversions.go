package cli

import "github.com/spf13/cobra"

func templateVersions() *cobra.Command {
	return &cobra.Command{
		Use:     "versions",
		Aliases: []string{"version"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}

// coder template versions
