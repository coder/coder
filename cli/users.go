package cli

import (
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) users() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short:   "Manage users",
		Use:     "users [subcommand]",
		Aliases: []string{"user"},
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.userCreate(),
			r.userList(),
			r.userSingle(),
			r.createUserStatusCommand(codersdk.UserStatusActive),
			r.createUserStatusCommand(codersdk.UserStatusSuspended),
		},
	}
	return cmd
}
