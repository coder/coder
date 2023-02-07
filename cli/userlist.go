package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func userList() *cobra.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.User{}, []string{"username", "email", "created_at", "status"}),
		cliui.JSONFormat(),
	)

	cmd := &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			res, err := client.Users(cmd.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}

			out, err := formatter.Format(cmd.Context(), res.Users)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	formatter.AttachFlags(cmd)
	return cmd
}

func userSingle() *cobra.Command {
	formatter := cliui.NewOutputFormatter(
		&userShowFormat{},
		cliui.JSONFormat(),
	)

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
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			user, err := client.User(cmd.Context(), args[0])
			if err != nil {
				return err
			}

			orgNames := make([]string, len(user.OrganizationIDs))
			for i, orgID := range user.OrganizationIDs {
				org, err := client.Organization(cmd.Context(), orgID)
				if err != nil {
					return xerrors.Errorf("get organization %q: %w", orgID.String(), err)
				}

				orgNames[i] = org.Name
			}

			out, err := formatter.Format(cmd.Context(), userWithOrgNames{
				User:              user,
				OrganizationNames: orgNames,
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	formatter.AttachFlags(cmd)
	return cmd
}

type userWithOrgNames struct {
	codersdk.User
	OrganizationNames []string `json:"organization_names"`
}

type userShowFormat struct{}

var _ cliui.OutputFormat = &userShowFormat{}

// ID implements OutputFormat.
func (*userShowFormat) ID() string {
	return "table"
}

// AttachFlags implements OutputFormat.
func (*userShowFormat) AttachFlags(_ *cobra.Command) {}

// Format implements OutputFormat.
func (*userShowFormat) Format(_ context.Context, out interface{}) (string, error) {
	user, ok := out.(userWithOrgNames)
	if !ok {
		return "", xerrors.Errorf("expected type %T, got %T", user, out)
	}

	tw := cliui.Table()
	addRow := func(name string, value interface{}) {
		key := ""
		if name != "" {
			key = name + ":"
		}
		tw.AppendRow(table.Row{
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
	for _, orgName := range user.OrganizationNames {
		key := ""
		if firstOrg {
			key = "Organizations"
			firstOrg = false
		}

		addRow(key, orgName)
	}
	if firstOrg {
		addRow("Organizations", "(none)")
	}

	return tw.Render(), nil
}
