package cli

import "github.com/spf13/cobra"

func parameterDelete() *cobra.Command {
	return &cobra.Command{
		Use:     "delete",
		Aliases: []string{"rm"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
