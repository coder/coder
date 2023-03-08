package cli

import (
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) users() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short:   "Manage users",
		Use:     "users",
		Aliases: []string{"user"},
		Handler: func(inv *clibase.Invokation) error {
			return inv.Command.HelpHandler(inv)
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
