package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) templateVersions() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:     "versions",
		Short:   "Manage different versions of the specified template",
		Aliases: []string{"version"},
		Long: formatExamples(
			example{
				Description: "List versions of a specific template",
				Command:     "coder templates versions list my-template",
			},
		),
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.templateVersionsList(),
		},
	}

	return cmd
}

func (r *RootCmd) templateVersionsList() *clibase.Cmd {
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateVersionRow{}, nil),
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)

	cmd := &clibase.Cmd{
		Use: "list <template>",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Short: "List all the versions of the specified template",
		Handler: func(inv *clibase.Invocation) error {
			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}
			req := codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			}

			versions, err := client.TemplateVersionsByTemplate(inv.Context(), req)
			if err != nil {
				return xerrors.Errorf("get template versions by template: %w", err)
			}

			rows := templateVersionsToRows(template.ActiveVersionID, versions...)
			out, err := formatter.Format(inv.Context(), rows)
			if err != nil {
				return xerrors.Errorf("render table: %w", err)
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}

	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type templateVersionRow struct {
	// For json format:
	TemplateVersion codersdk.TemplateVersion `table:"-"`

	// For table format:
	Name      string    `json:"-" table:"name,default_sort"`
	CreatedAt time.Time `json:"-" table:"created at"`
	CreatedBy string    `json:"-" table:"created by"`
	Status    string    `json:"-" table:"status"`
	Active    string    `json:"-" table:"active"`
}

// templateVersionsToRows converts a list of template versions to a list of rows
// for outputting.
func templateVersionsToRows(activeVersionID uuid.UUID, templateVersions ...codersdk.TemplateVersion) []templateVersionRow {
	rows := make([]templateVersionRow, len(templateVersions))
	for i, templateVersion := range templateVersions {
		activeStatus := ""
		if templateVersion.ID == activeVersionID {
			activeStatus = cliui.Keyword("Active")
		}

		rows[i] = templateVersionRow{
			TemplateVersion: templateVersion,
			Name:            templateVersion.Name,
			CreatedAt:       templateVersion.CreatedAt,
			CreatedBy:       templateVersion.CreatedBy.Username,
			Status:          strings.Title(string(templateVersion.Job.Status)),
			Active:          activeStatus,
		}
	}

	return rows
}
