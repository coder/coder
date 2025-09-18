package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) groupDelete() *serpent.Command {
	orgContext := agpl.NewOrganizationContext()
	cmd := &serpent.Command{
		Use:   "delete <name>",
		Short: "Delete a user group",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx       = inv.Context()
				groupName = inv.Args[0]
			)
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			org, err := orgContext.Selected(inv, client)
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
	orgContext.AttachOptions(cmd)

	return cmd
}
