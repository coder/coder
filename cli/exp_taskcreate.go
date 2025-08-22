package cli

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
	"github.com/google/uuid"
	"golang.org/x/xerrors"
)

func (r *RootCmd) taskCreate() *serpent.Command {
	var (
		orgContext = NewOrganizationContext()
		client     = new(codersdk.Client)

		templateName        string
		templateVersionName string
		presetName          string
		taskInput           string
	)

	return &serpent.Command{
		Use:   "create [task]",
		Short: "Create an experimental task",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			{
				Flag:     "input",
				Env:      "CODER_TASK_INPUT",
				Value:    serpent.StringOf(&taskInput),
				Required: true,
			},
			{
				Env:   "CODER_TEMPLATE_NAME",
				Value: serpent.StringOf(&templateName),
			},
			{
				Env:   "CODER_TEMPLATE_VERSION",
				Value: serpent.StringOf(&templateVersionName),
			},
			{
				Flag:    "preset",
				Env:     "CODER_PRESET_NAME",
				Value:   serpent.StringOf(&presetName),
				Default: PresetNone,
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			var (
				ctx       = inv.Context()
				expClient = codersdk.NewExperimentalClient(client)

				templateVersionID       uuid.UUID
				templateVersionPresetID uuid.UUID
			)

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			if len(inv.Args) > 0 {
				templateName, templateVersionName, _ = strings.Cut(inv.Args[0], "@")
			}

			if templateName == "" {
				templates, err := client.Templates(ctx, codersdk.TemplateFilter{SearchQuery: "has-ai-task:true"})
				if err != nil {
					return xerrors.Errorf("get templates: %w", err)
				}

				slices.SortFunc(templates, func(a, b codersdk.Template) int {
					return slice.Descending(a.ActiveUserCount, b.ActiveUserCount)
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				// If more than 1 organization exists in the list of templates,
				// then include the organization name in the select options.
				uniqueOrganizations := make(map[uuid.UUID]bool)
				for _, template := range templates {
					uniqueOrganizations[template.OrganizationID] = true
				}

				for _, template := range templates {
					templateName := template.Name
					if len(uniqueOrganizations) > 1 {
						templateName += cliui.Placeholder(
							fmt.Sprintf(
								" (%s)",
								template.OrganizationName,
							),
						)
					}

					if template.ActiveUserCount > 0 {
						templateName += cliui.Placeholder(
							fmt.Sprintf(
								" used by %s",
								formatActiveDevelopers(template.ActiveUserCount),
							),
						)
					}

					templateNames = append(templateNames, templateName)
					templateByName[templateName] = template
				}

				option, err := cliui.Select(inv, cliui.SelectOptions{
					Options:    templateNames,
					HideSearch: true,
				})

				templateName = templateByName[option].Name
			}

			if templateVersionName != "" {
				templateVersion, err := client.TemplateVersionByOrganizationAndName(ctx, organization.ID, templateName, templateVersionName)
				if err != nil {
					return xerrors.Errorf("get template version: %w", err)
				}

				templateVersionID = templateVersion.ID
			} else {
				template, err := client.TemplateByName(ctx, organization.ID, templateName)
				if err != nil {
					return xerrors.Errorf("get template: %w", err)
				}

				templateVersionID = template.ActiveVersionID
			}

			if presetName != PresetNone {
				templatePresets, err := client.TemplateVersionPresets(ctx, templateVersionID)
				if err != nil {
					return xerrors.Errorf("get template presets: %w", err)
				}

				preset, err := resolvePreset(templatePresets, presetName)
				if err != nil {
					return xerrors.Errorf("resolve preset: %w", err)
				}

				templateVersionID = preset.ID
			}

			workspace, err := expClient.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
				TemplateVersionID:       templateVersionID,
				TemplateVersionPresetID: templateVersionPresetID,
				Prompt:                  taskInput,
			})

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"The task %s has been created at %s!\n",
				cliui.Keyword(workspace.Name),
				cliui.Timestamp(time.Now()),
			)

			return nil
		},
	}
}
