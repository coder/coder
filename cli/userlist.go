package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) userList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.User{}, []string{"username", "email", "created at", "status"}),
		cliui.JSONFormat(),
	)
	var githubUserID int64

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "Prints the list of users.",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Options: serpent.OptionSet{
			{
				Name:        "github-user-id",
				Description: "Filter users by their GitHub user ID.",
				Default:     "",
				Flag:        "github-user-id",
				Required:    false,
				Value:       serpent.Int64Of(&githubUserID),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			req := codersdk.UsersRequest{}
			if githubUserID != 0 {
				req.Search = fmt.Sprintf("github_com_user_id:%d", githubUserID)
			}

			res, err := client.Users(inv.Context(), req)
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), res.Users)
			if err != nil {
				return err
			}

			if out == "" {
				cliui.Infof(inv.Stderr, "No users found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) userSingle() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		&userShowFormat{},
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:   "show <username|user_id|'me'>",
		Short: "Show a single user. Use 'me' to indicate the currently authenticated user.",
		Long: FormatExamples(
			Example{
				Command: "coder users show me",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

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

// userShowColumns are the fields userShowFormat can render, in default
// display order. Names match the table columns used by "users list" where
// the fields overlap ("username", "email", "status", "created at").
var userShowColumns = []string{"id", "username", "name", "email", "status", "created at", "roles", "organizations"}

type userShowFormat struct {
	columns []string
}

var _ cliui.OutputFormat = &userShowFormat{}

// ID implements OutputFormat.
func (*userShowFormat) ID() string {
	return "table"
}

// AttachOptions implements OutputFormat.
func (f *userShowFormat) AttachOptions(opts *serpent.OptionSet) {
	*opts = append(*opts,
		serpent.Option{
			Flag:          "column",
			FlagShorthand: "c",
			Default:       strings.Join(userShowColumns, ","),
			Value:         serpent.EnumArrayOf(&f.columns, userShowColumns...),
			Description:   "Columns to display in table output.",
		},
	)
}

// Format implements OutputFormat.
func (f *userShowFormat) Format(_ context.Context, out interface{}) (string, error) {
	user, ok := out.(userWithOrgNames)
	if !ok {
		return "", xerrors.Errorf("expected type %T, got %T", user, out)
	}

	show := make(map[string]bool, len(f.columns))
	for _, c := range f.columns {
		show[c] = true
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

	if show["id"] {
		addRow("ID", user.ID.String())
	}
	if show["username"] {
		addRow("Username", user.Username)
	}
	if show["name"] {
		addRow("Full name", user.Name)
	}
	if show["email"] {
		addRow("Email", user.Email)
	}
	if show["status"] {
		addRow("Status", user.Status)
	}
	if show["created at"] {
		addRow("Created At", user.CreatedAt.Format(time.Stamp))
	}

	if show["roles"] {
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
	}

	if show["organizations"] {
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
	}

	return tw.Render(), nil
}
