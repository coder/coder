package cli

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
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
		stdin               bool
		wait                bool
		waitInterval        time.Duration
		waitTimeout         time.Duration
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
				Name:        "wait",
				Flag:        "wait",
				Description: "Wait for task completion and stream real-time logs while running.",
				Value:       serpent.BoolOf(&wait),
			},
			{
				Name:        "wait-timeout",
				Flag:        "wait-timeout",
				Description: "How long to wait for task completion.",
				Value:       serpent.DurationOf(&waitTimeout),
			},
			{
				Name:        "wait-interval",
				Flag:        "wait-interval",
				Description: "Interval to poll the task for status updates. Only used in tests.",
				Hidden:      true,
				Value:       serpent.DurationOf(&waitInterval),
				Default:     "1s",
			},
		},
		Handler: func(inv *serpent.Invocation) error {
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

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"The task %s has been created at %s!\n",
				cliui.Keyword(task.Name),
				cliui.Timestamp(task.CreatedAt),
			)

			if !wait {
				return nil
			}

			var (
				eg      errgroup.Group
				agentID = make(chan uuid.UUID)
				done    = make(chan struct{})
			)

			eg.Go(func() error {
				defer close(done)

				var (
					agentIDSent bool
					waitCtx     = ctx
				)

				if waitTimeout != 0 {
					ctx, cancel := context.WithTimeout(waitCtx, waitTimeout)
					defer cancel()
					waitCtx = ctx
				}

				// TODO(DanielleMaywood):
				// Implement streaming updates instead of polling.
				t := time.NewTicker(waitInterval)
				defer t.Stop()
				for {
					select {
					case <-waitCtx.Done():
						return nil
					case <-t.C:
					}

					task, err := expClient.TaskByID(ctx, task.ID)
					if err != nil {
						return xerrors.Errorf("get task: %w", err)
					}

					if !agentIDSent && task.WorkspaceAgentID.Valid {
						agentID <- task.WorkspaceAgentID.UUID
						agentIDSent = true
					}

					if task.Status == codersdk.WorkspaceStatusStarting {
						continue
					}

					if task.CurrentState == nil {
						continue
					}

					if task.CurrentState.State == codersdk.TaskStateIdle ||
						task.CurrentState.State == codersdk.TaskStateCompleted {
						return nil
					}
				}
			})

			eg.Go(func() error {
				select {
				case <-ctx.Done():
					return nil
				case <-done:
					return nil

				case agentID := <-agentID:
					agentLogs, closer, err := client.WorkspaceAgentLogsAfter(ctx, agentID, 0, true)
					if err != nil {
						return xerrors.Errorf("follow agent logs: %w", err)
					}
					defer closer.Close()

					for {
						select {
						case <-ctx.Done():
							return nil

						case <-done:
							return nil

						case agentLogChunk := <-agentLogs:
							for _, agentLog := range agentLogChunk {
								fmt.Fprintln(inv.Stdout, agentLog.Output)
							}
						}
					}
				}
			})

			return eg.Wait()
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
