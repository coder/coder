package cli

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

// createUserStatusCommand sets a user status.
func (r *RootCmd) createUserStatusCommand(sdkStatus codersdk.UserStatus) *clibase.Cmd {
	var verb string
	var pastVerb string
	var aliases []string
	var short string
	switch sdkStatus {
	case codersdk.UserStatusActive:
		verb = "activate"
		pastVerb = "activated"
		aliases = []string{"active"}
		short = "Update a user's status to 'active'. Active users can fully interact with the platform"
	case codersdk.UserStatusSuspended:
		verb = "suspend"
		pastVerb = "suspended"
		aliases = []string{"rm", "delete"}
		short = "Update a user's status to 'suspended'. A suspended user cannot log into the platform"
	default:
		panic(fmt.Sprintf("%s is not supported", sdkStatus))
	}

	client := new(codersdk.Client)

	var columns []string
	cmd := &clibase.Cmd{
		Use:     fmt.Sprintf("%s <username|user_id>", verb),
		Short:   short,
		Aliases: aliases,
		Long: formatExamples(
			example{
				Command: fmt.Sprintf("coder users %s example_user", verb),
			},
		),
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			identifier := inv.Args[0]
			if identifier == "" {
				return xerrors.Errorf("user identifier cannot be an empty string")
			}

			user, err := client.User(inv.Context(), identifier)
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			// Display the user. This uses cliui.DisplayTable directly instead
			// of cliui.NewOutputFormatter because we prompt immediately
			// afterwards.
			table, err := cliui.DisplayTable([]codersdk.User{user}, "", columns)
			if err != nil {
				return xerrors.Errorf("render user table: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, table)

			// User status is already set to this
			if user.Status == sdkStatus {
				_, _ = fmt.Fprintf(inv.Stdout, "User status is already %q\n", sdkStatus)
				return nil
			}

			// Prompt to confirm the action
			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      fmt.Sprintf("Are you sure you want to %s this user?", verb),
				IsConfirm: true,
				Default:   cliui.ConfirmYes,
			})
			if err != nil {
				return err
			}

			_, err = client.UpdateUserStatus(inv.Context(), user.ID.String(), sdkStatus)
			if err != nil {
				return xerrors.Errorf("%s user: %w", verb, err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nUser %s has been %s!\n", cliui.DefaultStyles.Keyword.Render(user.Username), pastVerb)
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "column",
			FlagShorthand: "c",
			Description:   "Specify a column to filter in the table.",
			Default:       strings.Join([]string{"username", "email", "created_at", "status"}, ","),
			Value:         clibase.StringArrayOf(&columns),
		},
	}
	return cmd
}
