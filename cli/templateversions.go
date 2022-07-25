package cli

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templateVersions() *cobra.Command {
	cmd := &cobra.Command{
		Use:     "versions",
		Short:   "Manage different versions of the specified template",
		Aliases: []string{"version"},
		Example: formatExamples(
			example{
				Description: "List versions of a specific template",
				Command:     "coder templates versions list my-template",
			},
		),
	}
	cmd.AddCommand(
		templateVersionsList(),
	)

	return cmd
}

func templateVersionsList() *cobra.Command {
	return &cobra.Command{
		Use:   "list <template>",
		Args:  cobra.ExactArgs(1),
		Short: "List all the versions of the specified template",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(cmd.Context(), organization.ID, args[0])
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}
			req := codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			}

			versions, err := client.TemplateVersionsByTemplate(cmd.Context(), req)
			if err != nil {
				return xerrors.Errorf("get template versions by template: %w", err)
			}
			_, err = fmt.Fprintln(cmd.OutOrStdout(), displayTemplateVersions(template.ActiveVersionID, versions...))
			return err
		},
	}
}

// displayTemplateVersions will return a table displaying existing
// template versions for the specified template.
func displayTemplateVersions(activeVersionID uuid.UUID, templateVersions ...codersdk.TemplateVersion) string {
	tableWriter := cliui.Table()
	header := table.Row{
		"Name", "Created At", "Created By", "Status", ""}
	tableWriter.AppendHeader(header)
	for _, templateVersion := range templateVersions {
		var activeStatus = ""
		if templateVersion.ID == activeVersionID {
			activeStatus = cliui.Styles.Code.Render(cliui.Styles.Keyword.Render("Active"))
		}
		tableWriter.AppendRow(table.Row{
			templateVersion.Name,
			templateVersion.CreatedAt.Format("03:04:05 PM MST on Jan 2, 2006"),
			templateVersion.CreatedByName,
			strings.Title(string(templateVersion.Job.Status)),
			activeStatus,
		})
	}
	return tableWriter.Render()
}
