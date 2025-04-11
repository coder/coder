package cli

import (
	"sort"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
	"golang.org/x/xerrors"
)

func (r *RootCmd) userEditRoles() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:        "edit-roles <username|user_id>",
		Short:      "Edit a user's roles by username or id",
		Middleware: serpent.Chain(serpent.RequireNArgs(1), r.InitClient(client)),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			user, err := client.User(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			roles, err := client.ListSiteRoles(ctx)
			if err != nil {
				return xerrors.Errorf("fetch site roles: %w", err)
			}

			siteRoles := make([]string, 0)
			for _, role := range roles {
				if role.Assignable {
					siteRoles = append(siteRoles, role.Name)
				}
			}

			userRoles, err := client.UserRoles(ctx, user.Username)
			if err != nil {
				return xerrors.Errorf("fetch user roles: %w", err)
			}

			sort.Strings(siteRoles)
			selectedRoles, err := cliui.MultiSelect(inv, cliui.MultiSelectOptions{
				Message:  "Select the roles you'd like to assign to the user",
				Options:  siteRoles,
				Defaults: userRoles.Roles,
			})
			if err != nil {
				return xerrors.Errorf("selecting roles for user: %w", err)
			}

			_, err = client.UpdateUserRoles(ctx, user.Username, codersdk.UpdateRoles{
				Roles: selectedRoles,
			})
			if err != nil {
				return xerrors.Errorf("update user roles: %w", err)
			}

			return nil
		},
	}

	return cmd
}
