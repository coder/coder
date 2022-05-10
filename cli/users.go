package cli

import "github.com/spf13/cobra"

func users() *cobra.Command {
	cmd := &cobra.Command{
		Short: "Create, remove, and list users",
		Use:   "users",
	}
	cmd.AddCommand(userCreate(), userList())
	return cmd
}
