package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func templateList() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			templates, err := client.TemplatesByOrganization(cmd.Context(), organization.ID)
			if err != nil {
				return err
			}

			if len(templates) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s No templates found in %s! Create one:\n\n", caret, color.HiWhiteString(organization.Name))
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), color.HiMagentaString("  $ coder templates create <directory>\n"))
				return nil
			}

			tableWriter := cliui.Table()
			tableWriter.AppendHeader(table.Row{"Name", "Last updated", "Used by"})

			for _, template := range templates {
				suffix := ""
				if template.WorkspaceOwnerCount != 1 {
					suffix = "s"
				}
				tableWriter.AppendRow(table.Row{
					template.Name,
					template.UpdatedAt.Format("January 2, 2006"),
					cliui.Styles.Fuschia.Render(fmt.Sprintf("%d developer%s", template.WorkspaceOwnerCount, suffix)),
				})
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), tableWriter.Render())
			return err
		},
	}
}
