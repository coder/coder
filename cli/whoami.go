package cli

import (
	"fmt"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) whoami() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "whoami",
		Short:       "Fetch authenticated user info for Coder deployment",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			// Fetch the user info
			resp, err := client.User(ctx, codersdk.Me)
			// Get Coder instance url
			clientURL := client.URL

			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(inv.Stdout, Caret+"Coder is running at %s, You're authenticated as %s !\n", pretty.Sprint(cliui.DefaultStyles.Keyword, clientURL), pretty.Sprint(cliui.DefaultStyles.Keyword, resp.Username))
			return err
		},
	}
	return cmd
}
