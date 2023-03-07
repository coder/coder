package cli

import (
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
	"gvisor.dev/gvisor/runsc/cmd"
)

func users() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short:   "Manage users",
		Use:     "users",
		Aliases: []string{"user"},
		Handler: func(inv *clibase.Invokation) error {
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
