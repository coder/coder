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
		replaceRoles []string
		addRoles     []string
		removeRoles  []string
	)
	cmd := &serpent.Command{
		Use:   "edit-roles <username|user_id>",
		Short: "Edit a user's roles by username or id",
		Options: []serpent.Option{
			cliui.SkipPromptOption(),
			{
				Name:        "roles",
				Description: "A list of roles to give to the user. This replaces all existing roles. Use --add or --remove to modify roles incrementally.",
				Flag:        "roles",
				Value:       serpent.StringArrayOf(&replaceRoles),
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
			if len(replaceRoles) > 0 && (len(addRoles) > 0 || len(removeRoles) > 0) {
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
				// Validate --add and --remove roles against assignable
				// site roles so typos fail fast.
				for _, role := range addRoles {
					if !slices.Contains(siteRoleNames, role) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("role %q is not valid, assignable roles: %s", role, siteRolesPretty)
					}
				}
				for _, role := range removeRoles {
					if !slices.Contains(siteRoleNames, role) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("role %q is not valid, assignable roles: %s", role, siteRolesPretty)
					}
				}

				// Check permissions scoped to the operations
				// requested: only check assign when adding, only
				// check unassign when removing.
				checks := make(map[string]codersdk.AuthorizationCheck)
				if len(addRoles) > 0 {
					checks["assignRole"] = codersdk.AuthorizationCheck{
						Object: codersdk.AuthorizationObject{
							ResourceType: codersdk.ResourceAssignRole,
						},
						Action: codersdk.ActionAssign,
					}
				}
				if len(removeRoles) > 0 {
					checks["unassignRole"] = codersdk.AuthorizationCheck{
						Object: codersdk.AuthorizationObject{
							ResourceType: codersdk.ResourceAssignRole,
						},
						Action: codersdk.ActionUnassign,
					}
				}
				authResp, err := client.AuthCheck(ctx, codersdk.AuthorizationRequest{
					Checks: checks,
				})
				if err != nil {
					return xerrors.Errorf("check permissions: %w", err)
				}
				for check, allowed := range authResp {
					if !allowed {
						return xerrors.Errorf("you do not have permission to %s", check)
					}
				}

				// Start from the user's current roles, filtering
				// out the implied "member" role which is not a real
				// assignable site role.
				currentRoles := make([]string, 0, len(userRoles.Roles))
				for _, role := range userRoles.Roles {
					if role != "member" {
						currentRoles = append(currentRoles, role)
					}
				}
				selectedRoles = append([]string{}, currentRoles...)

				// Apply additions.
				for _, role := range addRoles {
					if !slices.Contains(selectedRoles, role) {
						selectedRoles = append(selectedRoles, role)
					}
				}

				// Apply removals.
				selectedRoles = slices.DeleteFunc(selectedRoles, func(role string) bool {
					return slices.Contains(removeRoles, role)
				})

				// If nothing changed, inform the user and exit early.
				slices.Sort(selectedRoles)
				slices.Sort(currentRoles)
				if slices.Equal(selectedRoles, currentRoles) {
					cliui.Infof(inv.Stdout, "No role changes required; the user already has the desired roles.")
					return nil
				}

			case len(replaceRoles) > 0:
				// Make sure all of the given roles are valid site roles
				for _, replaceRole := range replaceRoles {
					if !slices.Contains(siteRoleNames, replaceRole) {
						siteRolesPretty := strings.Join(siteRoleNames, ", ")
						return xerrors.Errorf("The role %s is not valid. Please use one or more of the following roles: %s\n", replaceRole, siteRolesPretty)
					}
				}

				selectedRoles = replaceRoles

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
