package cli

import (
	"fmt"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func templateVersions() *cobra.Command {
	return &cobra.Command{
		Use:     "versions [template]",
		Args:    cobra.ExactArgs(1),
		Short:   "List all the versions of the specified template",
		Aliases: []string{"version"},
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
			_, err = fmt.Fprintln(cmd.OutOrStdout(), displayTemplateVersions(versions...))
			return err
		},
	}
}

// displayTemplateVersions will return a table displaying existing
// template versions for the specified template.
func displayTemplateVersions(templateVersions ...codersdk.TemplateVersion) string {
	tableWriter := cliui.Table()
	header := table.Row{
		"Name", "Created At", "Created By"}
	tableWriter.AppendHeader(header)
	for _, templateVersion := range templateVersions {
		tableWriter.AppendRow(table.Row{
			templateVersion.Name,
			templateVersion.CreatedAt.Format("03:04:05PM MST on Jan 2, 2006"),
			templateVersion.CreatedByName,
		})
	}
	return tableWriter.Render()
}
