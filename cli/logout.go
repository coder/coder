package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
)

func logout() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "logout",
		Short: "Unauthenticate your local session",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			var errors []error

			config := createConfig(cmd)

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Are you sure you want to log out?",
				IsConfirm: true,
				Default:   cliui.ConfirmYes,
			})
			if err != nil {
				return err
			}

			err = client.Logout(cmd.Context())
			if err != nil {
				errors = append(errors, xerrors.Errorf("logout api: %w", err))
			}

			err = config.URL().Delete()
			// Only throw error if the URL configuration file is present,
			// otherwise the user is already logged out, and we proceed
			if err != nil && !os.IsNotExist(err) {
				errors = append(errors, xerrors.Errorf("remove URL file: %w", err))
			}

			err = config.Session().Delete()
			// Only throw error if the session configuration file is present,
			// otherwise the user is already logged out, and we proceed
			if err != nil && !os.IsNotExist(err) {
				errors = append(errors, xerrors.Errorf("remove session file: %w", err))
			}

			err = config.Organization().Delete()
			// If the organization configuration file is absent, we still proceed
			if err != nil && !os.IsNotExist(err) {
				errors = append(errors, xerrors.Errorf("remove organization file: %w", err))
			}

			if len(errors) > 0 {
				var errorStringBuilder strings.Builder
				for _, err := range errors {
					_, _ = fmt.Fprint(&errorStringBuilder, "\t"+err.Error()+"\n")
				}
				errorString := strings.TrimRight(errorStringBuilder.String(), "\n")
				return xerrors.New("Failed to log out.\n" + errorString)
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), Caret+"You are no longer logged in. You can log in using 'coder login <url>'.\n")
			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)
	return cmd
}
