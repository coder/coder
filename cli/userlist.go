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

func userSingle() *cobra.Command {
	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:     "show <username|user_id|'me'>",
		Short:   "Show a single user. Use 'me' to indicate the currently authenticated user.",
		Example: "coder users show me",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			user, err := client.User(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), displayUsers(columns, user))
			return err
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"username", "email", "created_at"},
		"Specify a column to filter in the table.")
	return cmd
}
