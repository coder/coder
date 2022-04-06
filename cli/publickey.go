package cli

import (
	"fmt"

	"github.com/coder/coder/codersdk"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func publickey() *cobra.Command {
	return &cobra.Command{
		Use: "publickey",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			key, err := client.GitSSHKey(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			fmt.Println(key)

			return nil
		},
	}
}
