package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

func (r *RootCmd) groupDelete() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:        "delete <name>",
		Short:      "Delete a user group",
		Middleware: clibase.RequireNArgs(1),
		Handler: func(inv *clibase.Invokation) error {
			var (
				ctx       = inv.Context()
				groupName = inv.Args[0]
			)

			client, err := agpl.CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			org, err := agpl.CurrentOrganization(inv, client)
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

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully deleted group %s!\n", cliui.Styles.Keyword.Render(group.Name))
			return nil
		},
	}

	return cmd
}
