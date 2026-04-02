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
	var (
		givenRoles  []string
		addRoles    []string
		removeRoles []string
	)
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
			{
				Name:        "add",
				Description: "A list of roles to add to the user's existing roles. Cannot be used together with --roles.",
				Flag:        "add",
				Value:       serpent.StringArrayOf(&addRoles),
			},
			{
				Name:        "remove",
				Description: "A list of roles to remove from the user's existing roles. Cannot be used together with --roles.",
				Flag:        "remove",
				Value:       serpent.StringArrayOf(&removeRoles),
			},
		},
		Middleware: serpent.Chain(serpent.RequireNArgs(1)),
		Handler: func(inv *serpent.Invocation) error {
			// Validate flag conflicts before any API calls.
			if len(givenRoles) > 0 && (len(addRoles) > 0 || len(removeRoles) > 0) {
				return xerrors.Errorf("--roles cannot be used with --add or --remove")
			}
			for _, role := range addRoles {
				if slices.Contains(removeRoles, role) {
					return xerrors.Errorf("role %q cannot appear in both --add and --remove", role)
				}
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()

			user, err := client.User(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			// Pre-flight check: verify the caller has permission to
			// assign site roles before we do any further work.
			authResp, err := client.AuthCheck(ctx, codersdk.AuthorizationRequest{
				Checks: map[string]codersdk.AuthorizationCheck{
					"assignRole": {
						Object: codersdk.AuthorizationObject{
							ResourceType: codersdk.ResourceAssignRole,
						},
						Action: codersdk.ActionAssign,
					},
				},
			})
			if err != nil {
				return xerrors.Errorf("check permissions: %w", err)
			}
			if !authResp["assignRole"] {
				return xerrors.Errorf("you do not have permission to edit user roles")
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
			switch {
			case len(addRoles) > 0 || len(removeRoles) > 0:
				// Validate --add roles against assignable site roles.
				for _, role := range addRoles {
					if !slices.Contains(siteRoleNames, role) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("The role %s is not valid. Please use one or more of the following roles: %s\n", role, siteRolesPretty)
					}
				}

				// Start from the user's current assignable roles,
				// filtering out implied roles like "member" that
				// are not real assignable site roles.
				currentAssignable := make([]string, 0, len(userRoles.Roles))
				for _, role := range userRoles.Roles {
					if slices.Contains(siteRoleNames, role) {
						currentAssignable = append(currentAssignable, role)
					}
				}
				selectedRoles = append([]string{}, currentAssignable...)

				// Apply additions.
				for _, role := range addRoles {
					if !slices.Contains(selectedRoles, role) {
						selectedRoles = append(selectedRoles, role)
					}
				}

				// Apply removals.
				selectedRoles = slices.DeleteFunc(selectedRoles, func(r string) bool {
					return slices.Contains(removeRoles, r)
				})

				// If nothing changed, inform the user and exit early.
				slices.Sort(selectedRoles)
				slices.Sort(currentAssignable)
				if slices.Equal(selectedRoles, currentAssignable) {
					cliui.Infof(inv.Stdout, "No role changes required; the user already has the desired roles.")
					return nil
				}

			case len(givenRoles) > 0:
				// Make sure all of the given roles are valid site roles
				for _, givenRole := range givenRoles {
					if !slices.Contains(siteRoleNames, givenRole) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("The role %s is not valid. Please use one or more of the following roles: %s\n", givenRole, siteRolesPretty)
					}
				}

				selectedRoles = givenRoles

			default:
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
