package cli

import "github.com/spf13/cobra"

func users() *cobra.Command {
	cmd := &cobra.Command{
		Use: "users",
	}
	return cmd
}
