package cli

import (
	"fmt"

	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/cli/cliui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func userStatus() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Update the status of a user",
	}
	cmd.AddCommand(
		setUserStatus(codersdk.UserStatusActive),
		setUserStatus(codersdk.UserStatusSuspended),
	)
	return cmd
}

// setUserStatus sets a user status.
func setUserStatus(sdkStatus codersdk.UserStatus) *cobra.Command {
	var verb string
	var aliases []string
	switch sdkStatus {
	case codersdk.UserStatusActive:
		verb = "active"
		aliases = []string{"activate"}
	case codersdk.UserStatusSuspended:
		verb = "suspend"
		aliases = []string{"rm", "delete"}
	default:
		panic(fmt.Sprintf("%s is not supported", sdkStatus))
	}

	var (
		columns []string
	)
	cmd := &cobra.Command{
		Use:     fmt.Sprintf("%s <username|user_id>", verb),
		Short:   fmt.Sprintf("Update a user's status to %q", sdkStatus),
		Args:    cobra.ExactArgs(1),
		Aliases: aliases,
		Example: fmt.Sprintf("coder users status %s example_user", verb),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			identifier := args[0]
			if identifier == "" {
				return xerrors.Errorf("user identifier cannot be an empty string")
			}

			user, err := client.UserByIdentifier(cmd.Context(), identifier)
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			// Display the user
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), DisplayUsers(columns, user))

			// User status is already set to this
			if user.Status == sdkStatus {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "User status is already %q\n", sdkStatus)
				return nil
			}

			// Prompt to confirm the action
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Are you sure you want to %s this user?", verb),
				IsConfirm: true,
				Default:   "yes",
			})
			if err != nil {
				return err
			}

			_, err = client.SetUserStatus(cmd.Context(), user.ID, sdkStatus)
			if err != nil {
				return xerrors.Errorf("%s user: %w", verb, err)
			}
			return nil
		},
	}
	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"Username", "Email", "Created At", "Status"},
		"Specify a column to filter in the table.")
	return cmd
}
