package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) taskCreate() *serpent.Command {
	var (
		orgContext = NewOrganizationContext()

		templateName        string
		templateVersionName string
		presetName          string
		stdin               bool
		quiet               bool
	)

	cmd := &serpent.Command{
		Use:   "create [input]",
		Short: "Create an experimental task",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
		),
		Options: serpent.OptionSet{
			{
				Name:  "template",
				Flag:  "template",
				Env:   "CODER_TASK_TEMPLATE_NAME",
				Value: serpent.StringOf(&templateName),
			},
			{
				Name:  "template-version",
				Flag:  "template-version",
				Env:   "CODER_TASK_TEMPLATE_VERSION",
				Value: serpent.StringOf(&templateVersionName),
			},
			{
				Name:    "preset",
				Flag:    "preset",
				Env:     "CODER_TASK_PRESET_NAME",
				Value:   serpent.StringOf(&presetName),
				Default: PresetNone,
			},
			{
				Name:        "stdin",
				Flag:        "stdin",
				Description: "Reads from stdin for the task input.",
				Value:       serpent.BoolOf(&stdin),
			},
			{
				Name:          "quiet",
				Flag:          "quiet",
				FlagShorthand: "q",
				Description:   "Only display the created task's ID.",
				Value:         serpent.BoolOf(&quiet),
			},
		},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			var (
				ctx       = inv.Context()
				expClient = codersdk.NewExperimentalClient(client)

				taskInput               string
				templateVersionID       uuid.UUID
				templateVersionPresetID uuid.UUID
			)

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			if stdin {
				bytes, err := io.ReadAll(inv.Stdin)
				if err != nil {
					return xerrors.Errorf("reading stdin: %w", err)
				}

				taskInput = string(bytes)
			} else {
				if len(inv.Args) != 1 {
					return xerrors.Errorf("expected an input for task")
				}

				taskInput = inv.Args[0]
			}

			if taskInput == "" {
				return xerrors.Errorf("a task cannot be started with an empty input")
			}

			switch {
			case templateName == "":
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
					templateNames := make([]string, 0, len(templates))
					for _, template := range templates {
						templateNames = append(templateNames, template.Name)
					}

					return xerrors.Errorf("template name not provided, available templates: %s", strings.Join(templateNames, ", "))
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

			case templateVersionName != "":
				templateVersion, err := client.TemplateVersionByOrganizationAndName(ctx, organization.ID, templateName, templateVersionName)
				if err != nil {
					return xerrors.Errorf("get template version: %w", err)
				}

				templateVersionID = templateVersion.ID

			default:
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

			if quiet {
				_, _ = fmt.Fprintln(inv.Stdout, task.ID)
			} else {
				_, _ = fmt.Fprintf(
					inv.Stdout,
					"The task %s has been created at %s!\n",
					cliui.Keyword(task.Name),
					cliui.Timestamp(task.CreatedAt),
				)
			}

			return nil
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
