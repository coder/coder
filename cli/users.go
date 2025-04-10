package cli

import (
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) users() *serpent.Command {
	cmd := &serpent.Command{
		Short:   "Manage users",
		Use:     "users [subcommand]",
		Aliases: []string{"user"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.userCreate(),
			r.userList(),
			r.userSingle(),
			r.userDelete(),
			r.userEditRoles(),
			r.createUserStatusCommand(codersdk.UserStatusActive),
			r.createUserStatusCommand(codersdk.UserStatusSuspended),
		},
	}
	return cmd
}

func (r *RootCmd) userEditRoles() *serpent.Command {
	orgContext := NewOrganizationContext()
	cmd := r.assignOrganizationRoles(orgContext)
	cmd.Short = "Edit a member's roles"

	return cmd
}
