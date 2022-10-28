package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func groupList() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List user groups",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx = cmd.Context()
			)

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			org, err := agpl.CurrentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			groups, err := client.GroupsByOrganization(ctx, org.ID)
			if err != nil {
				return xerrors.Errorf("get groups: %w", err)
			}

			if len(groups) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s No groups found in %s! Create one:\n\n", agpl.Caret, color.HiWhiteString(org.Name))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), color.HiMagentaString("  $ coder groups create <name>\n"))
				return nil
			}

			out, err := displayGroups(groups...)
			if err != nil {
				return xerrors.Errorf("display groups: %w", err)
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), out)
			return nil
		},
	}
	return cmd
}

type groupTableRow struct {
	Name           string    `table:"name"`
	OrganizationID uuid.UUID `table:"organization_id"`
	Members        []string  `table:"members"`
	AvatarURL      string    `table:"avatar_url"`
}

func displayGroups(groups ...codersdk.Group) (string, error) {
	rows := make([]groupTableRow, 0, len(groups))
	for _, group := range groups {
		members := make([]string, 0, len(group.Members))
		for _, member := range group.Members {
			members = append(members, member.Email)
		}
		rows = append(rows, groupTableRow{
			Name:           group.Name,
			OrganizationID: group.OrganizationID,
			AvatarURL:      group.AvatarURL,
			Members:        members,
		})
	}

	return cliui.DisplayTable(rows, "name", nil)
}
