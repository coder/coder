package cli
import (
	"errors"
	"fmt"
	"github.com/google/uuid"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func (r *RootCmd) createOrganization() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "create <organization name>",
		Short: "Create a new organization.",
		Middleware: serpent.Chain(
			r.InitClient(client),
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			orgName := inv.Args[0]
			err := codersdk.NameValid(orgName)
			if err != nil {
				return fmt.Errorf("organization name %q is invalid: %w", orgName, err)
			}
			// This check is not perfect since not all users can read all organizations.
			// So ignore the error and if the org already exists, prevent the user
			// from creating it.
			existing, _ := client.OrganizationByName(inv.Context(), orgName)
			if existing.ID != uuid.Nil {
				return fmt.Errorf("organization %q already exists", orgName)
			}
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text: fmt.Sprintf("Are you sure you want to create an organization with the name %s?\n%s",
					pretty.Sprint(cliui.DefaultStyles.Code, orgName),
					pretty.Sprint(cliui.BoldFmt(), "This action is irreversible."),
				),
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
				return fmt.Errorf("failed to create organization: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Organization %s (%s) created.\n", organization.Name, organization.ID)
			return nil
		},
	}
	return cmd
}
