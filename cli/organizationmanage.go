package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) createOrganization() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "create <organization name>",
		Short: "Create a new organization.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
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

			organization, err := client.CreateOrganization(inv.Context(), codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			if err != nil {
				return xerrors.Errorf("failed to create organization: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Organization %s (%s) created.\n", organization.Name, organization.ID)
			return nil
		},
	}

	return cmd
}

const defaultOrgMemberRolesFlag = "default-org-member-roles"

func (r *RootCmd) editOrganization(orgContext *OrganizationContext) *serpent.Command {
	var defaultOrgMemberRoles []string
	cmd := &serpent.Command{
		Use:   "edit",
		Short: "Edit organization settings.",
		Long: FormatExamples(
			Example{
				Description: "Replace the roles every member of the organization holds",
				Command:     "coder organizations edit --org <organization> --default-org-member-roles organization-workspace-access,organization-template-admin",
			},
			Example{
				Description: "Grant members no roles at all",
				Command:     `coder organizations edit --org <organization> --default-org-member-roles ""`,
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Options: serpent.OptionSet{
			{
				Name: defaultOrgMemberRolesFlag,
				Flag: defaultOrgMemberRolesFlag,
				Description: "Replaces the roles every member of the organization holds. " +
					"Accepts a comma-separated list and may be repeated. " +
					"An empty value removes every role. " +
					"New organizations start with organization-workspace-access, which grants members access to their own workspaces.",
				Value: serpent.StringArrayOf(&defaultOrgMemberRoles),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			if !inv.ParsedFlags().Changed(defaultOrgMemberRolesFlag) {
				return xerrors.Errorf("no changes requested; pass --%s", defaultOrgMemberRolesFlag)
			}

			roles, err := parseDefaultOrgMemberRoles(defaultOrgMemberRoles)
			if err != nil {
				return err
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}

			removed := make([]string, 0, len(org.DefaultOrgMemberRoles))
			for _, role := range org.DefaultOrgMemberRoles {
				if !slices.Contains(roles, role) {
					removed = append(removed, role)
				}
			}
			if len(removed) > 0 {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: fmt.Sprintf("Remove %s from every member of %s?",
						pretty.Sprint(cliui.DefaultStyles.Code, strings.Join(removed, ", ")),
						pretty.Sprint(cliui.DefaultStyles.Code, org.Name)),
					IsConfirm: true,
					Default:   cliui.ConfirmNo,
				})
				if err != nil {
					return err
				}
			}

			organization, err := client.UpdateOrganization(inv.Context(), org.ID.String(), codersdk.UpdateOrganizationRequest{
				DefaultOrgMemberRoles: &roles,
			})
			if err != nil {
				return xerrors.Errorf("failed to update organization: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Organization %s (%s) updated.\nDefault member roles: %s\n",
				organization.Name, organization.ID, formatOrgMemberRoles(organization.DefaultOrgMemberRoles))
			return nil
		},
	}
	orgContext.AttachOptions(cmd)

	return cmd
}

// parseDefaultOrgMemberRoles trims and de-duplicates the flag values,
// preserving the order they were given in. An empty set of values means
// "no roles": serpent resets the slice to nil when any flag value is
// empty, so `--flag a --flag ""` also lands here. The returned slice is
// always allocated so the request carries [] rather than null, which the
// server reads as "unspecified".
func parseDefaultOrgMemberRoles(values []string) ([]string, error) {
	roles := make([]string, 0, len(values))
	for _, role := range values {
		role = strings.TrimSpace(role)
		if role == "" {
			return nil, xerrors.Errorf("--%s contains an empty role name", defaultOrgMemberRolesFlag)
		}
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	return roles, nil
}

func formatOrgMemberRoles(roles []string) string {
	if len(roles) == 0 {
		return "(none)"
	}
	return strings.Join(roles, ", ")
}
