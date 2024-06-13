package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) organizationMembers() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "members",
		Aliases: []string{"member"},
		Short:   "Manage organization members",
		Children: []*serpent.Command{
			r.listOrganizationMembers(),
			r.assignOrganizationRoles(),
		},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}

	return cmd
}

func (r *RootCmd) assignOrganizationRoles() *serpent.Command {
	client := new(codersdk.Client)

	cmd := &serpent.Command{
		Use:     "edit-roles <username | user_id> [roles...]",
		Aliases: []string{"edit-role"},
		Short:   "Edit organization member's roles",
		Middleware: serpent.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			organization, err := CurrentOrganization(r, inv, client)
			if err != nil {
				return err
			}

			if len(inv.Args) < 1 {
				return xerrors.Errorf("user_id or username is required as the first argument")
			}
			userIdentifier := inv.Args[0]
			roles := inv.Args[1:]

			member, err := client.UpdateOrganizationMemberRoles(ctx, organization.ID, userIdentifier, codersdk.UpdateRoles{
				Roles: roles,
			})
			if err != nil {
				return xerrors.Errorf("update member roles: %w", err)
			}

			updatedTo := make([]string, 0)
			for _, role := range member.Roles {
				updatedTo = append(updatedTo, role.String())
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Member roles updated to [%s]\n", strings.Join(updatedTo, ", "))
			return nil
		},
	}

	return cmd
}

func (r *RootCmd) listOrganizationMembers() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.OrganizationMemberWithName{}, []string{"username", "organization_roles"}),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "list",
		Short: "List all organization members",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			organization, err := CurrentOrganization(r, inv, client)
			if err != nil {
				return err
			}

			res, err := client.OrganizationMembers(ctx, organization.ID)
			if err != nil {
				return xerrors.Errorf("fetch members: %w", err)
			}

			out, err := formatter.Format(inv.Context(), res)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)

	return cmd
}
