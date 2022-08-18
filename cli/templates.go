package cli

import (
	"fmt"
	"time"

	"github.com/google/uuid"
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

type templateTableRow struct {
	Name                 string                   `table:"name"`
	CreatedAt            string                   `table:"created at"`
	LastUpdated          string                   `table:"last updated"`
	OrganizationID       uuid.UUID                `table:"organization id"`
	Provisioner          codersdk.ProvisionerType `table:"provisioner"`
	ActiveVersionID      uuid.UUID                `table:"active version id"`
	UsedBy               string                   `table:"used by"`
	MaxTTL               time.Duration            `table:"max ttl"`
	MinAutostartInterval time.Duration            `table:"min autostart"`
}

// displayTemplates will return a table displaying all templates passed in.
// filterColumns must be a subset of the template fields and will determine which
// columns to display
func displayTemplates(filterColumns []string, templates ...codersdk.Template) (string, error) {
	rows := make([]templateTableRow, len(templates))
	for i, template := range templates {
		suffix := ""
		if template.WorkspaceOwnerCount != 1 {
			suffix = "s"
		}

		rows[i] = templateTableRow{
			Name:                 template.Name,
			CreatedAt:            template.CreatedAt.Format("January 2, 2006"),
			LastUpdated:          template.UpdatedAt.Format("January 2, 2006"),
			OrganizationID:       template.OrganizationID,
			Provisioner:          template.Provisioner,
			ActiveVersionID:      template.ActiveVersionID,
			UsedBy:               cliui.Styles.Fuchsia.Render(fmt.Sprintf("%d developer%s", template.WorkspaceOwnerCount, suffix)),
			MaxTTL:               (time.Duration(template.MaxTTLMillis) * time.Millisecond),
			MinAutostartInterval: (time.Duration(template.MinAutostartIntervalMillis) * time.Millisecond),
		}
	}

	return cliui.DisplayTable(rows, "name", filterColumns)
}
