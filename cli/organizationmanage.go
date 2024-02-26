package cli

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

func (r *RootCmd) createOrganization() *clibase.Cmd {
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use:   "create <organization name>",
		Short: "Create a new organization.",
		Middleware: clibase.Chain(
			r.InitClient(client),
			clibase.RequireNArgs(1),
		),
		Options: clibase.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *clibase.Invocation) error {
			orgName := inv.Args[0]

			// This check is not perfect since not all users can read all organizations.
			// So ignore the error and if the org already exists, prevent the user
			// from creating it.
			existing, _ := client.OrganizationByName(inv.Context(), orgName)
			if existing.ID != uuid.Nil {
				return fmt.Errorf("organization %q already exists", orgName)
			}

			// Confirm deletion of the template.
			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Are you sure you want to create an organization with the name %q?", pretty.Sprint(cliui.DefaultStyles.Code, orgName)),
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			organization, err := client.CreateOrganization(inv.Context(), codersdk.CreateOrganizationRequest{
				Name: orgName,
			})
			if err != nil {
				return fmt.Errorf("failed to create organization: %w")
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Organization %s (%s) created.\n", organization.Name, organization.ID)
			return nil
		},
	}

	return cmd
}
