package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
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
			start := time.Now()
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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Templates found in %s %s\n\n",
				caret,
				color.HiWhiteString(organization.Name),
				color.HiBlackString("[%dms]",
					time.Since(start).Milliseconds()))

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 4, ' ', 0)
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
				color.HiBlackString("Template"),
				color.HiBlackString("Source"),
				color.HiBlackString("Last Updated"),
				color.HiBlackString("Used By"))
			for _, template := range templates {
				suffix := ""
				if template.WorkspaceOwnerCount != 1 {
					suffix = "s"
				}
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
					color.New(color.FgHiCyan).Sprint(template.Name),
					color.WhiteString("Archive"),
					color.WhiteString(template.UpdatedAt.Format("January 2, 2006")),
					color.New(color.FgHiWhite).Sprintf("%d developer%s", template.WorkspaceOwnerCount, suffix))
			}
			return writer.Flush()
		},
	}
}
