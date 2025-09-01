package cli

import (
	"fmt"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
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

	cmd := &serpent.Command{
		Use:   "create [input]",
		Short: "Create an experimental task",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			{
				Flag:  "input",
				Env:   "CODER_TASK_INPUT",
				Value: serpent.StringOf(&taskInput),
			},
			{
				Flag:  "template",
				Env:   "CODER_TASK_TEMPLATE_NAME",
				Value: serpent.StringOf(&templateName),
			},
			{
				Flag:  "template-version",
				Env:   "CODER_TASK_TEMPLATE_VERSION",
				Value: serpent.StringOf(&templateVersionName),
			},
			{
				Flag:    "preset",
				Env:     "CODER_TASK_PRESET_NAME",
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
				taskInput = inv.Args[0]
			}

			if templateName == "" {
				templates, err := client.Templates(ctx, codersdk.TemplateFilter{SearchQuery: "has-ai-task:true", OrganizationID: organization.ID})
				if err != nil {
					return xerrors.Errorf("list templates: %w", err)
				}

				if len(templates) == 0 {
					return xerrors.Errorf("no task templates configured")
				}

				// When a deployment has only 1 AI task template, we will
				// allow omitting the template. Otherwise we will require
				// the user to be explicit with their choice of template.
				if len(templates) > 1 {
					return xerrors.Errorf("template name not provided")
				}

				if templateVersionName != "" {
					templateVersion, err := client.TemplateVersionByOrganizationAndName(ctx, organization.ID, templates[0].Name, templateVersionName)
					if err != nil {
						return xerrors.Errorf("get template version: %w", err)
					}

					templateVersionID = templateVersion.ID
				} else {
					templateVersionID = templates[0].ActiveVersionID
				}
			} else if templateVersionName != "" {
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

				templateVersionPresetID = preset.ID
			}

			task, err := expClient.CreateTask(ctx, codersdk.Me, codersdk.CreateTaskRequest{
				TemplateVersionID:       templateVersionID,
				TemplateVersionPresetID: templateVersionPresetID,
				Prompt:                  taskInput,
			})
			if err != nil {
				return xerrors.Errorf("create task: %w", err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"The task %s has been created at %s!\n",
				cliui.Keyword(task.Name),
				cliui.Timestamp(task.CreatedAt),
			)

			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
