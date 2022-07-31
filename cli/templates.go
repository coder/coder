package cli

import (
	"fmt"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Short:   "Create, manage, and deploy templates",
		Aliases: []string{"template"},
		Example: formatExamples(
			example{
				Description: "Create a template for developers to create workspaces",
				Command:     "coder templates create",
			},
			example{
				Description: "Make changes to your template, and plan the changes",
				Command:     "coder templates plan my-template",
			},
			example{
				Description: "Push an update to the template. Your developers can update their workspaces",
				Command:     "coder templates push my-template",
			},
		),
	}
	cmd.AddCommand(
		templateCreate(),
		templateEdit(),
		templateInit(),
		templateList(),
		templatePlan(),
		templatePush(),
		templateVersions(),
		templateDelete(),
		templatePull(),
	)

	return cmd
}

// displayTemplates will return a table displaying all templates passed in.
// filterColumns must be a subset of the template fields and will determine which
// columns to display
func displayTemplates(filterColumns []string, templates ...codersdk.Template) string {
	tableWriter := cliui.Table()
	header := table.Row{
		"Name", "Created At", "Last Updated", "Organization ID", "Provisioner",
		"Active Version ID", "Used By", "Max TTL", "Min Autostart"}
	tableWriter.AppendHeader(header)
	tableWriter.SetColumnConfigs(cliui.FilterTableColumns(header, filterColumns))
	tableWriter.SortBy([]table.SortBy{{
		Name: "name",
	}})
	for _, template := range templates {
		suffix := ""
		if template.WorkspaceOwnerCount != 1 {
			suffix = "s"
		}
		tableWriter.AppendRow(table.Row{
			template.Name,
			template.CreatedAt.Format("January 2, 2006"),
			template.UpdatedAt.Format("January 2, 2006"),
			template.OrganizationID.String(),
			template.Provisioner,
			template.ActiveVersionID.String(),
			cliui.Styles.Fuchsia.Render(fmt.Sprintf("%d developer%s", template.WorkspaceOwnerCount, suffix)),
			(time.Duration(template.MaxTTLMillis) * time.Millisecond).String(),
			(time.Duration(template.MinAutostartIntervalMillis) * time.Millisecond).String(),
		})
	}
	return tableWriter.Render()
}
