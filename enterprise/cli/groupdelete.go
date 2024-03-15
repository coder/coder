package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) groupDelete() *serpent.Cmd {
	client := new(codersdk.Client)
	cmd := &serpent.Cmd{
		Use:   "delete <name>",
		Short: "Delete a user group",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx       = inv.Context()
				groupName = inv.Args[0]
			)

			org, err := agpl.CurrentOrganization(&r.RootCmd, inv, client)
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

			_, _ = fmt.Fprintf(inv.Stdout, "Successfully deleted group %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, group.Name))
			return nil
		},
	}

	return cmd
}
