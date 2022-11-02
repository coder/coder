package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
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
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
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
			client, err := CreateClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}
			organization, err := CurrentOrganization(cmd, client)
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

			out, err := displayTemplateVersions(template.ActiveVersionID, versions...)
			if err != nil {
				return xerrors.Errorf("render table: %w", err)
			}

			_, err = fmt.Fprintln(cmd.OutOrStdout(), out)
			return err
		},
	}
}

type templateVersionRow struct {
	Name      string    `table:"name"`
	CreatedAt time.Time `table:"created at"`
	CreatedBy string    `table:"created by"`
	Status    string    `table:"status"`
	Active    string    `table:"active"`
}

// displayTemplateVersions will return a table displaying existing
// template versions for the specified template.
func displayTemplateVersions(activeVersionID uuid.UUID, templateVersions ...codersdk.TemplateVersion) (string, error) {
	rows := make([]templateVersionRow, len(templateVersions))
	for i, templateVersion := range templateVersions {
		var activeStatus = ""
		if templateVersion.ID == activeVersionID {
			activeStatus = cliui.Styles.Code.Render(cliui.Styles.Keyword.Render("Active"))
		}

		rows[i] = templateVersionRow{
			Name:      templateVersion.Name,
			CreatedAt: templateVersion.CreatedAt,
			CreatedBy: templateVersion.CreatedBy.Username,
			Status:    strings.Title(string(templateVersion.Job.Status)),
			Active:    activeStatus,
		}
	}

	return cliui.DisplayTable(rows, "name", nil)
}
