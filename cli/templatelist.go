package cli

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateList() *clibase.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateTableRow{}, []string{"name", "last updated", "used by"}),
		cliui.JSONFormat(),
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "list",
		Short:   "List all the templates available for the organization",
		Aliases: []string{"ls"},
		Middleware: clibase.Chain(
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}
			templates, err := client.TemplatesByOrganization(inv.Context(), organization.ID)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				_, _ = fmt.Fprintf(inv.Stderr, "%s No templates found in %s! Create one:\n\n", Caret, color.HiWhiteString(organization.Name))
				_, _ = fmt.Fprintln(inv.Stderr, color.HiMagentaString("  $ coder templates push <directory>\n"))
				return nil
			}

			rows := templatesToRows(templates...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
