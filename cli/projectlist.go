package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

func projectList() *cobra.Command {
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
			projects, err := client.ProjectsByOrganization(cmd.Context(), organization.ID)
			if err != nil {
				return err
			}

			if len(projects) == 0 {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s No projects found in %s! Create one:\n\n", caret, color.HiWhiteString(organization.Name))
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), color.HiMagentaString("  $ coder projects create <directory>\n"))
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Projects found in %s %s\n\n",
				caret,
				color.HiWhiteString(organization.Name),
				color.HiBlackString("[%dms]",
					time.Since(start).Milliseconds()))

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 4, ' ', 0)
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
				color.HiBlackString("Project"),
				color.HiBlackString("Source"),
				color.HiBlackString("Last Updated"),
				color.HiBlackString("Used By"))
			for _, project := range projects {
				suffix := ""
				if project.WorkspaceOwnerCount != 1 {
					suffix = "s"
				}
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\n",
					color.New(color.FgHiCyan).Sprint(project.Name),
					color.WhiteString("Archive"),
					color.WhiteString(project.UpdatedAt.Format("January 2, 2006")),
					color.New(color.FgHiWhite).Sprintf("%d developer%s", project.WorkspaceOwnerCount, suffix))
			}
			return writer.Flush()
		},
	}
}
