package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) updateUserEmail() *serpent.Command {
	var (
		oldEmail string
		newEmail string
	)

	cmd := &serpent.Command{
		Use:    "update-user-email",
		Short:  "Update a user's email address (break-glass; experimental)",
		Hidden: true,
		Options: serpent.OptionSet{
			{
				Flag:        "old-email",
				Description: "Current email address of the user to update.",
				Required:    true,
				Value:       serpent.StringOf(&oldEmail),
			},
			{
				Flag:        "new-email",
				Description: "New email address to assign to the user.",
				Required:    true,
				Value:       serpent.StringOf(&newEmail),
			},
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			if oldEmail == "" {
				return xerrors.Errorf("--old-email must not be blank")
			}
			if newEmail == "" {
				return xerrors.Errorf("--new-email must not be blank")
			}

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout,
				"This will update the email address for the account currently using %q to %q.\n"+
					"All Coder sessions and API tokens for that user will be revoked.\n"+
					"If the user logs in with an external identity provider, it may overwrite the email when the user next signs in.\n",
				oldEmail, newEmail,
			)

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm email update?",
				IsConfirm: true,
				Default:   cliui.ConfirmNo,
			})
			if err != nil {
				return err
			}

			err = client.UpdateUserEmail(inv.Context(), codersdk.UpdateUserEmailRequest{
				OldEmail: oldEmail,
				NewEmail: newEmail,
			})
			if err != nil {
				return xerrors.Errorf("update user email: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "Updated user email from %s to %s.\n", oldEmail, newEmail)
			return nil
		},
	}

	return cmd
}
