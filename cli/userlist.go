package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) userList() *serpent.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.User{}, []string{"username", "email", "created_at", "status"}),
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)

	cmd := &serpent.Cmd{
		Use:     "list",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			res, err := client.Users(inv.Context(), codersdk.UsersRequest{})
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), res.Users)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) userSingle() *serpent.Cmd {
	formatter := cliui.NewOutputFormatter(
		&userShowFormat{},
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)

	cmd := &serpent.Cmd{
		Use:   "show <username|user_id|'me'>",
		Short: "Show a single user. Use 'me' to indicate the currently authenticated user.",
		Long: formatExamples(
			example{
				Command: "coder users show me",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			user, err := client.User(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}

			orgNames := make([]string, len(user.OrganizationIDs))
			for i, orgID := range user.OrganizationIDs {
				org, err := client.Organization(inv.Context(), orgID)
				if err != nil {
					return xerrors.Errorf("get organization %q: %w", orgID.String(), err)
				}

				orgNames[i] = org.Name
			}

			out, err := formatter.Format(inv.Context(), userWithOrgNames{
				User:              user,
				OrganizationNames: orgNames,
			})
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
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

// AttachOptions implements OutputFormat.
func (*userShowFormat) AttachOptions(_ *serpent.OptionSet) {}

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
