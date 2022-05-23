package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func logout() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove the local authenticated session",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := createConfig(cmd)

			loginHelper := "You are not logged in. Try logging in using 'coder login <url>'."

			err := config.URL().Delete()

			if err != nil {
				// If the URL configuration file is absent, the user is logged out
				if os.IsNotExist(err) {
					return xerrors.New(loginHelper)
				}
				return xerrors.Errorf("remove URL file: %w", err)
			}

			err = config.Session().Delete()

			if err != nil {
				// If the session configuration file is absent, the user is logged out
				if os.IsNotExist(err) {
					return xerrors.New(loginHelper)
				}
				return xerrors.Errorf("remove session file: %w", err)
			}

			err = config.Organization().Delete()

			// If the organization configuration file is absent, we should still log out
			if err != nil && !os.IsNotExist(err) {
				return xerrors.Errorf("remove organization file: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), caret+"Successfully logged out.\n")
			return nil
		},
	}
}
