package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func templateList() *cobra.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateTableRow{}, []string{"name", "last updated", "used by"}),
		cliui.JSONFormat(),
	)

	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List all the templates available for the organization",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return err
			}
			templates, err := client.TemplatesByOrganization(cmd.Context(), organization.ID)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "%s No templates found in %s! Create one:\n\n", Caret, color.HiWhiteString(organization.Name))
				_, _ = fmt.Fprintln(cmd.ErrOrStderr(), color.HiMagentaString("  $ coder templates create <directory>\n"))
				return nil
			}

			rows := templatesToRows(templates...)
			out, err := formatter.Format(cmd.Context(), rows)
			if err != nil {
				return err
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}

	formatter.AttachFlags(cmd)
	return cmd
}
