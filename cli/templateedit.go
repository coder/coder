package cli

import "github.com/spf13/cobra"

func templateEdit() *cobra.Command {
	return &cobra.Command{
		Use: "edit",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
