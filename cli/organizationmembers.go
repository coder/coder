package cli
import (
	"errors"
	"fmt"
	"strings"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) organizationMembers(orgContext *OrganizationContext) *serpent.Command {
	cmd := &serpent.Command{
		Use:     "members",
		Aliases: []string{"member"},
		Short:   "Manage organization members",
		Children: []*serpent.Command{
			r.listOrganizationMembers(orgContext),
			r.assignOrganizationRoles(orgContext),
			r.addOrganizationMember(orgContext),
			r.removeOrganizationMember(orgContext),
		},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
	}
	return cmd
}
func (r *RootCmd) removeOrganizationMember(orgContext *OrganizationContext) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "remove <username | user_id>",
		Short: "Remove a new member to the current organization",
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			user := inv.Args[0]
			err = client.DeleteOrganizationMember(ctx, organization.ID, user)
			if err != nil {
				return fmt.Errorf("could not remove member from organization %q: %w", organization.HumanName(), err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Organization member removed from %q\n", organization.HumanName())
			return nil
		},
	}
	return cmd
}
func (r *RootCmd) addOrganizationMember(orgContext *OrganizationContext) *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "add <username | user_id>",
		Short: "Add a new member to the current organization",
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			user := inv.Args[0]
			_, err = client.PostOrganizationMember(ctx, organization.ID, user)
			if err != nil {
				return fmt.Errorf("could not add member to organization %q: %w", organization.HumanName(), err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Organization member added to %q\n", organization.HumanName())
			return nil
		},
	}
	return cmd
}
func (r *RootCmd) assignOrganizationRoles(orgContext *OrganizationContext) *serpent.Command {
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
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			if len(inv.Args) < 1 {
				return fmt.Errorf("user_id or username is required as the first argument")
			}
			userIdentifier := inv.Args[0]
			roles := inv.Args[1:]
			member, err := client.UpdateOrganizationMemberRoles(ctx, organization.ID, userIdentifier, codersdk.UpdateRoles{
				Roles: roles,
			})
			if err != nil {
				return fmt.Errorf("update member roles: %w", err)
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
func (r *RootCmd) listOrganizationMembers(orgContext *OrganizationContext) *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.OrganizationMemberWithUserData{}, []string{"username", "organization roles"}),
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
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			res, err := client.OrganizationMembers(ctx, organization.ID)
			if err != nil {
				return fmt.Errorf("fetch members: %w", err)
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
