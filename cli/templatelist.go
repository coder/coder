package cli

import (
	"fmt"

	"github.com/fatih/color"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) templateList() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateTableRow{}, []string{"name", "organization name", "last updated", "used by"}),
		cliui.JSONFormat(),
	)

	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List all the templates available for the organization",
		Aliases: []string{"ls"},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			templates, err := client.Templates(inv.Context(), codersdk.TemplateFilter{})
			if err != nil {
				return err
			}

			rows := templatesToRows(templates...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return err
			}

			if out == "" {
				_, _ = fmt.Fprintf(inv.Stderr, "%s No templates found! Create one:\n\n", Caret)
				_, _ = fmt.Fprintln(inv.Stderr, color.HiMagentaString("  $ coder templates push <directory>\n"))
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}
