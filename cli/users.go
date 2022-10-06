package cli

import (
	"github.com/spf13/cobra"

	"github.com/coder/coder/codersdk"
)

func users() *cobra.Command {
	cmd := &cobra.Command{
		Short:   "Manage users",
		Use:     "users",
		Aliases: []string{"user"},
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		userCreate(),
		userList(),
		userSingle(),
		createUserStatusCommand(codersdk.UserStatusActive),
		createUserStatusCommand(codersdk.UserStatusSuspended),
	)
	return cmd
}
