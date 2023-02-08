package cli

import (
	"time"

	"github.com/google/uuid"
	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templates() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "templates",
		Short:   "Manage templates",
		Long:    "Templates are written in standard Terraform and describe the infrastructure for workspaces",
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
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
	// Used by json format:
	Template codersdk.Template

	// Used by table format:
	Name            string                   `json:"-" table:"name,default_sort"`
	CreatedAt       string                   `json:"-" table:"created at"`
	LastUpdated     string                   `json:"-" table:"last updated"`
	OrganizationID  uuid.UUID                `json:"-" table:"organization id"`
	Provisioner     codersdk.ProvisionerType `json:"-" table:"provisioner"`
	ActiveVersionID uuid.UUID                `json:"-" table:"active version id"`
	UsedBy          string                   `json:"-" table:"used by"`
	DefaultTTL      time.Duration            `json:"-" table:"default ttl"`
}

// templateToRows converts a list of templates to a list of templateTableRow for
// outputting.
func templatesToRows(templates ...codersdk.Template) []templateTableRow {
	rows := make([]templateTableRow, len(templates))
	for i, template := range templates {
		rows[i] = templateTableRow{
			Template:        template,
			Name:            template.Name,
			CreatedAt:       template.CreatedAt.Format("January 2, 2006"),
			LastUpdated:     template.UpdatedAt.Format("January 2, 2006"),
			OrganizationID:  template.OrganizationID,
			Provisioner:     template.Provisioner,
			ActiveVersionID: template.ActiveVersionID,
			UsedBy:          cliui.Styles.Fuchsia.Render(formatActiveDevelopers(template.ActiveUserCount)),
			DefaultTTL:      (time.Duration(template.DefaultTTLMillis) * time.Millisecond),
		}
	}

	return rows
}
