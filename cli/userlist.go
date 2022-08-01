package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func userList() *cobra.Command {
	var (
		columns      []string
		outputFormat string
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

			out := ""
			switch outputFormat {
			case "table", "":
				out = displayUsers(columns, users...)
			case "json":
				outBytes, err := json.Marshal(users)
				if err != nil {
					return xerrors.Errorf("marshal users to JSON: %w", err)
				}

				out = string(outBytes)
			default:
				return xerrors.Errorf(`unknown output format %q, only "table" and "json" are supported`, outputFormat)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	cmd.Flags().StringArrayVarP(&columns, "column", "c", []string{"username", "email", "created_at", "status"},
		"Specify a column to filter in the table. Available columns are: id, username, email, created_at, status.")
	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format. Available formats are: table, json.")
	return cmd
}

func userSingle() *cobra.Command {
	var outputFormat string
	cmd := &cobra.Command{
		Use:   "show <username|user_id|'me'>",
		Short: "Show a single user. Use 'me' to indicate the currently authenticated user.",
		Example: formatExamples(
			example{
				Command: "coder users show me",
			},
		),
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			user, err := client.User(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			out := ""
			switch outputFormat {
			case "table", "":
				out = displayUser(cmd.Context(), cmd.ErrOrStderr(), client, user)
			case "json":
				outBytes, err := json.Marshal(user)
				if err != nil {
					return xerrors.Errorf("marshal user to JSON: %w", err)
				}

				out = string(outBytes)
			default:
				return xerrors.Errorf(`unknown output format %q, only "table" and "json" are supported`, outputFormat)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	cmd.Flags().StringVarP(&outputFormat, "output", "o", "table", "Output format. Available formats are: table, json.")
	return cmd
}

func displayUser(ctx context.Context, stderr io.Writer, client *codersdk.Client, user codersdk.User) string {
	tableWriter := cliui.Table()
	addRow := func(name string, value interface{}) {
		key := ""
		if name != "" {
			key = name + ":"
		}
		tableWriter.AppendRow(table.Row{
			key, value,
		})
	}

	// Add rows for each of the user's fields.
	addRow("ID", user.ID.String())
	addRow("Username", user.Username)
	addRow("Email", user.Email)
	addRow("Status", user.Status)
	addRow("Created At", user.CreatedAt.Format(time.Stamp))

	addRow("", "")
	firstRole := true
	for _, role := range user.Roles {
		if role.DisplayName == "" {
			// Skip roles with no display name.
			continue
		}

		key := ""
		if firstRole {
			key = "Roles"
			firstRole = false
		}
		addRow(key, role.DisplayName)
	}
	if firstRole {
		addRow("Roles", "(none)")
	}

	addRow("", "")
	firstOrg := true
	for _, orgID := range user.OrganizationIDs {
		org, err := client.Organization(ctx, orgID)
		if err != nil {
			warn := cliui.Styles.Warn.Copy().Align(lipgloss.Left)
			_, _ = fmt.Fprintf(stderr, warn.Render("Could not fetch organization %s: %+v"), orgID, err)
			continue
		}

		key := ""
		if firstOrg {
			key = "Organizations"
			firstOrg = false
		}

		addRow(key, org.Name)
	}
	if firstOrg {
		addRow("Organizations", "(none)")
	}

	return tableWriter.Render()
}
