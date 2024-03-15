package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) groupList() *serpent.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]groupTableRow{}, nil),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)
	cmd := &serpent.Cmd{
		Use:   "list",
		Short: "List user groups",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			org, err := agpl.CurrentOrganization(&r.RootCmd, inv, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			groups, err := client.GroupsByOrganization(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("get groups: %w", err)
			}

			if len(groups) == 0 {
				_, _ = fmt.Fprintf(inv.Stderr, "%s No groups found in %s! Create one:\n\n", agpl.Caret, color.HiWhiteString(org.Name))
				_, _ = fmt.Fprintln(inv.Stderr, color.HiMagentaString("  $ coder groups create <name>\n"))
				return nil
			}

			rows := groupsToRows(groups...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return xerrors.Errorf("display groups: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type groupTableRow struct {
	// For json output:
	Group codersdk.Group `table:"-"`

	// For table output:
	Name           string    `json:"-" table:"name,default_sort"`
	DisplayName    string    `json:"-" table:"display_name"`
	OrganizationID uuid.UUID `json:"-" table:"organization_id"`
	Members        []string  `json:"-" table:"members"`
	AvatarURL      string    `json:"-" table:"avatar_url"`
}

func groupsToRows(groups ...codersdk.Group) []groupTableRow {
	rows := make([]groupTableRow, 0, len(groups))
	for _, group := range groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.Email)
		}
		rows = append(rows, groupTableRow{
			Name:           group.Name,
			DisplayName:    group.DisplayName,
			OrganizationID: group.OrganizationID,
			AvatarURL:      group.AvatarURL,
			Members:        members,
		})
	}

	return rows
}
