package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) templateVersions() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "versions",
		Short:   "Manage different versions of the specified template",
		Aliases: []string{"version"},
		Long: FormatExamples(
			Example{
				Description: "List versions of a specific template",
				Command:     "coder templates versions list my-template",
			},
		),
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.templateVersionsList(),
			r.archiveTemplateVersion(),
			r.unarchiveTemplateVersion(),
		},
	}

	return cmd
}

func (r *RootCmd) templateVersionsList() *serpent.Command {
	defaultColumns := []string{
		"Name",
		"Created At",
		"Created By",
		"Status",
		"Active",
	}
	formatter := cliui.NewOutputFormatter(
		cliui.TableFormat([]templateVersionRow{}, defaultColumns),
		cliui.JSONFormat(),
	)
	client := new(codersdk.Client)
	orgContext := NewOrganizationContext()

	var includeArchived serpent.Bool

	cmd := &serpent.Command{
		Use: "list <template>",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
			func(next serpent.HandlerFunc) serpent.HandlerFunc {
				return func(i *serpent.Invocation) error {
					// This is the only way to dynamically add the "archived"
					// column if '--include-archived' is true.
					// It does not make sense to show this column if the
					// flag is false.
					if includeArchived {
						for _, opt := range i.Command.Options {
							if opt.Flag == "column" {
								if opt.ValueSource == serpent.ValueSourceDefault {
									v, ok := opt.Value.(*serpent.StringArray)
									if ok {
										// Add the extra new default column.
										*v = append(*v, "Archived")
									}
								}
								break
							}
						}
					}
					return next(i)
				}
			},
		),
		Short: "List all the versions of the specified template",
		Options: serpent.OptionSet{
			{
				Name:        "include-archived",
				Description: "Include archived versions in the result list.",
				Flag:        "include-archived",
				Value:       &includeArchived,
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}
			template, err := client.TemplateByName(inv.Context(), organization.ID, inv.Args[0])
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}
			req := codersdk.TemplateVersionsByTemplateRequest{
				TemplateID:      template.ID,
				IncludeArchived: includeArchived.Value(),
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

	orgContext.AttachOptions(cmd)
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
	Archived  string    `json:"-" table:"archived"`
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

		archivedStatus := ""
		if templateVersion.Archived {
			archivedStatus = pretty.Sprint(cliui.DefaultStyles.Warn, "Archived")
		}

		rows[i] = templateVersionRow{
			TemplateVersion: templateVersion,
			Name:            templateVersion.Name,
			CreatedAt:       templateVersion.CreatedAt,
			CreatedBy:       templateVersion.CreatedBy.Username,
			Status:          strings.Title(string(templateVersion.Job.Status)),
			Active:          activeStatus,
			Archived:        archivedStatus,
		}
	}

	return rows
}
