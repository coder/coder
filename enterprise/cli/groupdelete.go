package cli
import (
	"errors"
	"fmt"
	agpl "github.com/coder/coder/v2/cli"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func (r *RootCmd) groupDelete() *serpent.Command {
	orgContext := agpl.NewOrganizationContext()
	client := new(codersdk.Client)
	cmd := &serpent.Command{
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
			org, err := orgContext.Selected(inv, client)
			if err != nil {
				return fmt.Errorf("current organization: %w", err)
			}
			group, err := client.GroupByOrgAndName(ctx, org.ID, groupName)
			if err != nil {
				return fmt.Errorf("group by org and name: %w", err)
			}
			err = client.DeleteGroup(ctx, group.ID)
			if err != nil {
				return fmt.Errorf("delete group: %w", err)
			}
			_, _ = fmt.Fprintf(inv.Stdout, "Successfully deleted group %s!\n", pretty.Sprint(cliui.DefaultStyles.Keyword, group.Name))
			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
