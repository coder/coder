package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/coder/coder/codersdk"
)

func userList() *cobra.Command {
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			users, err := client.Users(cmd.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), displayUsers(columns, users...))
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"username", "email", "created_at"},
		"Specify a column to filter in the table.")
	return cmd
}
