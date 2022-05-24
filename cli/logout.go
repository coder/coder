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
		Short: "Remove local autheticated session",
		RunE: func(cmd *cobra.Command, args []string) error {
			config := createConfig(cmd)
			err := os.RemoveAll(string(config))
			if err != nil {
				return xerrors.Errorf("remove files at %s: %w", config, err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), caret+"Successfully logged out.\n")
			return nil
		},
	}
}
