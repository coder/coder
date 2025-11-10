package cli

import (
	"fmt"
	"os"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/serpent"
)

func (r *RootCmd) logout() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "logout",
		Short: "Unauthenticate your local session",
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var errors []error

			config := r.createConfig()

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Are you sure you want to log out?",
				IsConfirm: true,
				Default:   cliui.ConfirmYes,
			})
			if err != nil {
				return err
			}

			err = client.Logout(inv.Context())
			if err != nil {
				errors = append(errors, xerrors.Errorf("logout api: %w", err))
			}

			err = config.URL().Delete()
			// Only throw error if the URL configuration file is present,
			// otherwise the user is already logged out, and we proceed
			if err != nil && !os.IsNotExist(err) {
				errors = append(errors, xerrors.Errorf("remove URL file: %w", err))
			}

			err = r.ensureTokenBackend().Delete(client.URL)
			// Only throw error if the session configuration file is present,
			// otherwise the user is already logged out, and we proceed
			if err != nil && !xerrors.Is(err, os.ErrNotExist) {
				if xerrors.Is(err, sessionstore.ErrNotImplemented) {
					errors = append(errors, errKeyringNotSupported)
				} else {
					errors = append(errors, xerrors.Errorf("remove session token: %w", err))
				}
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
			_, _ = fmt.Fprint(inv.Stdout, Caret+"You are no longer logged in. You can log in using 'coder login <url>'.\n")
			return nil
		},
	}
	cmd.Options = append(cmd.Options, cliui.SkipPromptOption())
	return cmd
}
