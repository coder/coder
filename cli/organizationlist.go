package cli

import (
	"fmt"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) listOrganizations() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]codersdk.Organization{}, []string{"name", "id", "default"}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List all organizations",
		Long:    "List all organizations. Requires a role which grants ResourceOrganization: read.",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			organizations, err := client.Organizations(inv.Context())
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), organizations)
			if err != nil {
				return err
			}

			if out == "" {
				cliui.Infof(inv.Stderr, "No organizations found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
