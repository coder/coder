package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) organizationMembers() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.OrganizationMemberWithName{}, []string{"username", "organization_roles"}),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:     "members",
		Short:   "List all organization members",
		Aliases: []string{"member"},
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
