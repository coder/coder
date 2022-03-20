package cli

import (
	"fmt"
	"text/tabwriter"
	"time"

	"github.com/fatih/color"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func workspaceList() *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			start := time.Now()
			workspaces, err := client.WorkspacesByUser(cmd.Context(), "")
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"No workspaces found! Create one:")
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render("coder workspaces create <name>"))
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				return nil
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Workspaces found %s\n\n",
				caret,
				color.HiBlackString("[%dms]",
					time.Since(start).Milliseconds()))

			writer := tabwriter.NewWriter(cmd.OutOrStdout(), 0, 0, 4, ' ', 0)
			_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%s\n",
				color.HiBlackString("Workspace"),
				color.HiBlackString("Project"),
				color.HiBlackString("Status"),
				color.HiBlackString("Last Built"),
				color.HiBlackString("Outdated"))
			for _, workspace := range workspaces {
				_, _ = fmt.Fprintf(writer, "%s\t%s\t%s\t%s\t%+v\n",
					color.New(color.FgHiCyan).Sprint(workspace.Name),
					color.WhiteString(workspace.ProjectName),
					color.WhiteString(string(workspace.LatestBuild.Transition)),
					color.WhiteString(workspace.LatestBuild.Job.CompletedAt.Format("January 2, 2006")),
					workspace.Outdated)
			}
			return writer.Flush()
		},
	}
}
