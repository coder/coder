package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
)

func groupDelete() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "delete <name>",
		Short: "Delete a user group",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				ctx       = cmd.Context()
				groupName = args[0]
			)

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			org, err := agpl.CurrentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("current organization: %w", err)
			}

			group, err := client.GroupByOrgAndName(ctx, org.ID, groupName)
			if err != nil {
				return xerrors.Errorf("group by org and name: %w", err)
			}

			err = client.DeleteGroup(ctx, group.ID)
			if err != nil {
				return xerrors.Errorf("delete group: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Successfully deleted group %s!\n", cliui.Styles.Keyword.Render(group.Name))
			return nil
		},
	}

	return cmd
}
