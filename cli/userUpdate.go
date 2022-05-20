package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func userResetGitSSH() *cobra.Command {
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:   "regen-ssh <username|user_id|'me'>",
		Short: "Generates a new ssh key for the user. The old ssh key will be deleted.",
		Long: "Generates a new ssh key for the user. The old ssh key will be deleted. " +
			"Use 'me' to indicate the currently authenticated user. The command outputs " +
			"public key of the new ssh key.",
		Example: "coder users regen-ssh me",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			gitSSH, err := client.RegenerateGitSSHKey(ctx, args[0])
			if err != nil {
				return err
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), gitSSH.PublicKey)
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"username", "email", "created_at"},
		"Specify a column to filter in the table.")
	return cmd
}
