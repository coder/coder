package cli
import (
	"errors"
	"fmt"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)
func (r *RootCmd) userDelete() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "delete <username|user_id>",
		Short: "Delete a user by username or user_id.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			user, err := client.User(ctx, inv.Args[0])
			if err != nil {
				return fmt.Errorf("fetch user: %w", err)
			}
			err = client.DeleteUser(ctx, user.ID)
			if err != nil {
				return fmt.Errorf("delete user: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stderr,
				"Successfully deleted "+pretty.Sprint(cliui.DefaultStyles.Keyword, user.Username)+".",
			)
			return nil
		},
	}
	return cmd
}
