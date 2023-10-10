package cli

import (
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/codersdk"
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
			r.userDelete(),
			r.createUserStatusCommand(codersdk.UserStatusActive),
			r.createUserStatusCommand(codersdk.UserStatusSuspended),
		},
	}
	return cmd
}
