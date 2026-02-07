package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) deleteOrganization(_ *OrganizationContext) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "delete <organization_name_or_id>",
		Short: "Delete an organization",
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

			orgArg := inv.Args[0]
			organization, err := client.OrganizationByName(inv.Context(), orgArg)
			if err != nil {
				return err
			}

			if organization.IsDefault {
				return xerrors.Errorf("cannot delete the default organization %q", organization.Name)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Delete organization %s?", pretty.Sprint(cliui.DefaultStyles.Code, organization.Name)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			err = client.DeleteOrganization(inv.Context(), organization.ID.String())
			if err != nil {
				return xerrors.Errorf("delete organization %q: %w", organization.Name, err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"Deleted organization %s at %s\n",
				pretty.Sprint(cliui.DefaultStyles.Keyword, organization.Name),
				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}

	return cmd
}
