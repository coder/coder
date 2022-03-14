package cli

import "github.com/spf13/cobra"

func configSSH() *cobra.Command {
	return &cobra.Command{
		Use: "config-ssh",
		RunE: func(cmd *cobra.Command, args []string) error {
			return nil
		},
	}
}
