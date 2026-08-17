package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) createOrganization() *serpent.Command {
	var defaultOrgMemberRoles []string
	cmd := &serpent.Command{
		Use:   "create <organization name>",
		Short: "Create a new organization.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			{
				Name:        "default-org-member-roles",
				Flag:        "default-org-member-roles",
				Description: "Roles granted to every member of the organization. Pass an empty string to grant no roles.",
				Value:       serpent.StringArrayOf(&defaultOrgMemberRoles),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			orgName := inv.Args[0]

			err = codersdk.NameValid(orgName)
			if err != nil {
				return xerrors.Errorf("organization name %q is invalid: %w", orgName, err)
			}

			// This check is not perfect since not all users can read all organizations.
			// So ignore the error and if the org already exists, prevent the user
			// from creating it.
			existing, _ := client.OrganizationByName(inv.Context(), orgName)
			if existing.ID != uuid.Nil {
				return xerrors.Errorf("organization %q already exists", orgName)
			}

			req := codersdk.CreateOrganizationRequest{
				Name: orgName,
			}
			if inv.ParsedFlags().Changed("default-org-member-roles") {
				// An empty flag value parses to a nil slice. Send an
				// allocated empty slice so the request carries [] rather
				// than null, which the server reads as "unspecified".
				roles := make([]string, 0, len(defaultOrgMemberRoles))
				for _, role := range defaultOrgMemberRoles {
					if role != "" {
						roles = append(roles, role)
					}
				}
				req.DefaultOrgMemberRoles = &roles
			}

			organization, err := client.CreateOrganization(inv.Context(), req)
			if err != nil {
				return xerrors.Errorf("failed to create organization: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Organization %s (%s) created.\n", organization.Name, organization.ID)
			return nil
		},
	}

	return cmd
}
