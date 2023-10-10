package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

func (r *RootCmd) userDelete() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "delete <username|user_id>",
		Short: "Delete a user by username or user_id.",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			user, err := client.User(ctx, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("fetch user: %w", err)
			}

			err = client.DeleteUser(ctx, user.ID)
			if err != nil {
				return xerrors.Errorf("delete user: %w", err)
			}

			_, _ = fmt.Fprintln(inv.Stderr,
				"Successfully deleted "+pretty.Sprint(cliui.DefaultStyles.Keyword, user.Username)+".",
			)
			return nil
		},
	}
	return cmd
}
