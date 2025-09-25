package cli

import (
	"fmt"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

type whoamiRow struct {
	URL      string `json:"url" table:"URL,default_sort"`
	Username string `json:"username" table:"Username"`
}

func (r whoamiRow) String() string {
	return fmt.Sprintf(
		Caret+"Coder is running at %s, You're authenticated as %s !\n",
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.URL),
		pretty.Sprint(cliui.DefaultStyles.Keyword, r.Username),
	)
}

func (r *RootCmd) whoami() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TextFormat(),
		cliui.JSONFormat(),
		cliui.TableFormat([]whoamiRow{}, []string{"url", "username"}),
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "whoami",
		Short:       "Fetch authenticated user info for Coder deployment",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()
			// Fetch the user info
			resp, err := client.User(ctx, codersdk.Me)
			// Get Coder instance url
			clientURL := client.URL
			if err != nil {
				return err
			}

			out, err := formatter.Format(ctx, []whoamiRow{
				{
					URL:      clientURL.String(),
					Username: resp.Username,
				},
			})
			if err != nil {
				return err
			}
			_, err = inv.Stdout.Write([]byte(out))
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}
