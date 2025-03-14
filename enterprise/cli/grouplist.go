package cli
import (
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/google/uuid"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)
func (r *RootCmd) groupList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]groupTableRow{}, nil),
		cliui.JSONFormat(),
	)
	orgContext := agpl.NewOrganizationContext()
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "list",
		Short: "List user groups",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return fmt.Errorf("current organization: %w", err)
			}
			groups, err := client.GroupsByOrganization(ctx, org.ID)
			if err != nil {
				return fmt.Errorf("get groups: %w", err)
			}
			if len(groups) == 0 {
				_, _ = fmt.Fprintf(inv.Stderr, "%s No groups found in %s! Create one:\n\n", agpl.Caret, color.HiWhiteString(org.Name))
				_, _ = fmt.Fprintln(inv.Stderr, color.HiMagentaString("  $ coder groups create <name>\n"))
				return nil
			}
			rows := groupsToRows(groups...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return fmt.Errorf("display groups: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stdout, out)
			return nil
		},
	}
	formatter.AttachOptions(&cmd.Options)
	orgContext.AttachOptions(cmd)
	return cmd
}
type groupTableRow struct {
	// For json output:
	Group codersdk.Group `table:"-"`
	// For table output:
	Name           string    `json:"-" table:"name,default_sort"`
	DisplayName    string    `json:"-" table:"display name"`
	OrganizationID uuid.UUID `json:"-" table:"organization id"`
	Members        []string  `json:"-" table:"members"`
	AvatarURL      string    `json:"-" table:"avatar url"`
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
