package cli

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) userEditRoles() *serpent.Command {
	var givenRoles []string
	cmd := &serpent.Command{
		Use:   "edit-roles <username|user_id>",
		Short: "Edit a user's roles by username or id",
		Long: FormatExamples(
			Example{
				Description: "--roles replaces the user's full set of site roles; any role not listed is removed",
				Command:     "coder users edit-roles example_user --roles owner user-admin",
			},
		),
		Options: []serpent.Option{
			cliui.SkipPromptOption(),
			{
				Name:        "roles",
				Description: "Replaces the user's full set of site roles with this list. Any existing role not included here is removed.",
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
			skipPrompt, _ := inv.ParsedFlags().GetBool("yes")

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
				if skipPrompt {
					return xerrors.Errorf("--roles is required when using --yes; the interactive role picker cannot be used non-interactively")
				}

				selectedRoles, err = cliui.MultiSelect(inv, cliui.MultiSelectOptions{
					Message:  "Select the roles you'd like to assign to the user",
					Options:  siteRoleNames,
					Defaults: userRoles.Roles,
				})
				if err != nil {
					return xerrors.Errorf("selecting roles for user: %w", err)
				}
			}

			added, removed := diffRoles(userRoles.Roles, selectedRoles)
			if len(added) == 0 && len(removed) == 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "No role changes for %s.\n", user.Username)
				return nil
			}

			if len(added) > 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Roles to add: %s\n", strings.Join(added, ", "))
			}
			if len(removed) > 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Roles to remove: %s\n", pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(removed, ", ")))
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("This replaces the full set of site roles for %s. Continue?", user.Username),
				IsConfirm: true,
				Default:   cliui.ConfirmYes,
			})
			if err != nil {
				return err
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

func diffRoles(current, next []string) (added, removed []string) {
	for _, role := range next {
		if !slices.Contains(current, role) {
			added = append(added, role)
		}
	}
	for _, role := range current {
		if !slices.Contains(next, role) {
			removed = append(removed, role)
		}
	}
	return added, removed
}
