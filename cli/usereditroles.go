package cli

import (
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) userEditRoles() *serpent.Command {
	var givenRoles []string
	cmd := &serpent.Command{
		Use:   "edit-roles <username|user_id>",
		Short: "Edit a user's roles by username or id",
		Options: []serpent.Option{
			cliui.SkipPromptOption(),
			{
				Name:        "roles",
				Description: "A list of roles to give to the user. This removes any existing roles the user may have.",
				Flag:        "roles",
				Value:       serpent.StringArrayOf(&givenRoles),
			},
		},
		Middleware: serpent.Chain(serpent.RequireNArgs(1)),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			user, err := client.User(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			userRoles, err := client.UserRoles(ctx, user.Username)
			if err != nil {
				return xerrors.Errorf("fetch user roles: %w", err)
			}
			siteRoles, err := client.ListSiteRoles(ctx)
			if err != nil {
				return xerrors.Errorf("fetch site roles: %w", err)
			}
			siteRoleNames := make([]string, 0, len(siteRoles))
			for _, role := range siteRoles {
				siteRoleNames = append(siteRoleNames, role.Name)
			}

			var selectedRoles []string
			if len(givenRoles) > 0 {
				// Make sure all of the given roles are valid site roles
				for _, givenRole := range givenRoles {
					if !slices.Contains(siteRoleNames, givenRole) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("The role %s is not valid. Please use one or more of the following roles: %s\n", givenRole, siteRolesPretty)
					}
				}

				selectedRoles = givenRoles
			} else {
				selectedRoles, err = cliui.MultiSelect(inv, cliui.MultiSelectOptions{
					Message:  "Select the roles you'd like to assign to the user",
					Options:  siteRoleNames,
					Defaults: userRoles.Roles,
				})
				if err != nil {
					return xerrors.Errorf("selecting roles for user: %w", err)
				}
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
