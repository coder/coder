package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/google/uuid"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) create() *serpent.Command {
	var (
		templateName  string
		startAt       string
		stopAfter     time.Duration
		workspaceName string

		parameterFlags     workspaceParameterFlags
		autoUpdates        string
		copyParametersFrom string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "create [name]",
		Short:       "Create a workspace",
		Long: formatExamples(
			example{
				Description: "Create a workspace for another user (if you have permission)",
				Command:     "coder create <username>/<workspace_name>",
			},
		),
		Middleware: serpent.Chain(r.InitClient(client)),
		Handler: func(inv *serpent.Invocation) error {
			organization, err := CurrentOrganization(r, inv, client)
			if err != nil {
				return err
			}

			workspaceOwner := codersdk.Me
			if len(inv.Args) >= 1 {
				workspaceOwner, workspaceName, err = splitNamedWorkspace(inv.Args[0])
				if err != nil {
					return err
				}
			}

			if workspaceName == "" {
				workspaceName, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Specify a name for your workspace:",
					Validate: func(workspaceName string) error {
						_, err = client.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
						if err == nil {
							return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}

			_, err = client.WorkspaceByOwnerAndName(inv.Context(), workspaceOwner, workspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			var sourceWorkspace codersdk.Workspace
			if copyParametersFrom != "" {
				sourceWorkspaceOwner, sourceWorkspaceName, err := splitNamedWorkspace(copyParametersFrom)
				if err != nil {
					return err
				}

				sourceWorkspace, err = client.WorkspaceByOwnerAndName(inv.Context(), sourceWorkspaceOwner, sourceWorkspaceName, codersdk.WorkspaceOptions{})
				if err != nil {
					return xerrors.Errorf("get source workspace: %w", err)
				}

				_, _ = fmt.Fprintf(inv.Stdout, "Coder will use the same template %q as the source workspace.\n", sourceWorkspace.TemplateName)
				templateName = sourceWorkspace.TemplateName
			}

			var template codersdk.Template
			var templateVersionID uuid.UUID
			if templateName == "" {
				_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Wrap, "Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.TemplatesByOrganization(inv.Context(), organization.ID)
				if err != nil {
					return err
				}

				slices.SortFunc(templates, func(a, b codersdk.Template) int {
					return slice.Descending(a.ActiveUserCount, b.ActiveUserCount)
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				for _, template := range templates {
					templateName := template.Name

					if template.ActiveUserCount > 0 {
						templateName += cliui.Placeholder(
							fmt.Sprintf(
								" (used by %s)",
								formatActiveDevelopers(template.ActiveUserCount),
							),
						)
					}

					templateNames = append(templateNames, templateName)
					templateByName[templateName] = template
				}

				// Move the cursor up a single line for nicer display!
				option, err := cliui.Select(inv, cliui.SelectOptions{
					Options:    templateNames,
					HideSearch: true,
				})
				if err != nil {
					return err
				}

				template = templateByName[option]
				templateVersionID = template.ActiveVersionID
			} else if sourceWorkspace.LatestBuild.TemplateVersionID != uuid.Nil {
				template, err = client.Template(inv.Context(), sourceWorkspace.TemplateID)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
				templateVersionID = sourceWorkspace.LatestBuild.TemplateVersionID
			} else {
				template, err = client.TemplateByName(inv.Context(), organization.ID, templateName)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
				templateVersionID = template.ActiveVersionID
			}

			var schedSpec *string
			if startAt != "" {
				sched, err := parseCLISchedule(startAt)
				if err != nil {
					return err
				}
				schedSpec = ptr.Ref(sched.String())
			}

			cliBuildParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
			if err != nil {
				return xerrors.Errorf("can't parse given parameter values: %w", err)
			}

			cliBuildParameterDefaults, err := asWorkspaceBuildParameters(parameterFlags.richParameterDefaults)
			if err != nil {
				return xerrors.Errorf("can't parse given parameter defaults: %w", err)
			}

			var sourceWorkspaceParameters []codersdk.WorkspaceBuildParameter
			if copyParametersFrom != "" {
				sourceWorkspaceParameters, err = client.WorkspaceBuildParameters(inv.Context(), sourceWorkspace.LatestBuild.ID)
				if err != nil {
					return xerrors.Errorf("get source workspace build parameters: %w", err)
				}
			}

			richParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
				Action:            WorkspaceCreate,
				TemplateVersionID: templateVersionID,
				NewWorkspaceName:  workspaceName,

				RichParameterFile:     parameterFlags.richParameterFile,
				RichParameters:        cliBuildParameters,
				RichParameterDefaults: cliBuildParameterDefaults,

				SourceWorkspaceParameters: sourceWorkspaceParameters,
			})
			if err != nil {
				return xerrors.Errorf("prepare build: %w", err)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm create?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			var ttlMillis *int64
			if stopAfter > 0 {
				ttlMillis = ptr.Ref(stopAfter.Milliseconds())
			}

			workspace, err := client.CreateWorkspace(inv.Context(), organization.ID, workspaceOwner, codersdk.CreateWorkspaceRequest{
				TemplateVersionID:   templateVersionID,
				Name:                workspaceName,
				AutostartSchedule:   schedSpec,
				TTLMillis:           ttlMillis,
				RichParameterValues: richParameters,
				AutomaticUpdates:    codersdk.AutomaticUpdates(autoUpdates),
			})
			if err != nil {
				return xerrors.Errorf("create workspace: %w", err)
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, workspace.LatestBuild.ID)
			if err != nil {
				return xerrors.Errorf("watch build: %w", err)
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s workspace has been created at %s!\n",
				cliui.Keyword(workspace.Name),
				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	cmd.Options = append(cmd.Options,
		serpent.Option{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_TEMPLATE_NAME",
			Description:   "Specify a template name.",
			Value:         serpent.StringOf(&templateName),
		},
		serpent.Option{
			Flag:        "start-at",
			Env:         "CODER_WORKSPACE_START_AT",
			Description: "Specify the workspace autostart schedule. Check coder schedule start --help for the syntax.",
			Value:       serpent.StringOf(&startAt),
		},
		serpent.Option{
			Flag:        "stop-after",
			Env:         "CODER_WORKSPACE_STOP_AFTER",
			Description: "Specify a duration after which the workspace should shut down (e.g. 8h).",
			Value:       serpent.DurationOf(&stopAfter),
		},
		serpent.Option{
			Flag:        "automatic-updates",
			Env:         "CODER_WORKSPACE_AUTOMATIC_UPDATES",
			Description: "Specify automatic updates setting for the workspace (accepts 'always' or 'never').",
			Default:     string(codersdk.AutomaticUpdatesNever),
			Value:       serpent.StringOf(&autoUpdates),
		},
		serpent.Option{
			Flag:        "copy-parameters-from",
			Env:         "CODER_WORKSPACE_COPY_PARAMETERS_FROM",
			Description: "Specify the source workspace name to copy parameters from.",
			Value:       serpent.StringOf(&copyParametersFrom),
		},
		cliui.SkipPromptOption(),
	)
	cmd.Options = append(cmd.Options, parameterFlags.cliParameters()...)
	cmd.Options = append(cmd.Options, parameterFlags.cliParameterDefaults()...)
	return cmd
}

type prepWorkspaceBuildArgs struct {
	Action            WorkspaceCLIAction
	TemplateVersionID uuid.UUID
	NewWorkspaceName  string

	LastBuildParameters       []codersdk.WorkspaceBuildParameter
	SourceWorkspaceParameters []codersdk.WorkspaceBuildParameter

	PromptBuildOptions bool
	BuildOptions       []codersdk.WorkspaceBuildParameter

	PromptRichParameters  bool
	RichParameters        []codersdk.WorkspaceBuildParameter
	RichParameterFile     string
	RichParameterDefaults []codersdk.WorkspaceBuildParameter
}

// prepWorkspaceBuild will ensure a workspace build will succeed on the latest template version.
// Any missing params will be prompted to the user. It supports rich parameters.
func prepWorkspaceBuild(inv *serpent.Invocation, client *codersdk.Client, args prepWorkspaceBuildArgs) ([]codersdk.WorkspaceBuildParameter, error) {
	ctx := inv.Context()

	templateVersion, err := client.TemplateVersion(ctx, args.TemplateVersionID)
	if err != nil {
		return nil, xerrors.Errorf("get template version: %w", err)
	}

	templateVersionParameters, err := client.TemplateVersionRichParameters(inv.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	parameterFile := map[string]string{}
	if args.RichParameterFile != "" {
		parameterFile, err = parseParameterMapFile(args.RichParameterFile)
		if err != nil {
			return nil, xerrors.Errorf("can't parse parameter map file: %w", err)
		}
	}

	resolver := new(ParameterResolver).
		WithLastBuildParameters(args.LastBuildParameters).
		WithSourceWorkspaceParameters(args.SourceWorkspaceParameters).
		WithPromptBuildOptions(args.PromptBuildOptions).
		WithBuildOptions(args.BuildOptions).
		WithPromptRichParameters(args.PromptRichParameters).
		WithRichParameters(args.RichParameters).
		WithRichParametersFile(parameterFile).
		WithRichParametersDefaults(args.RichParameterDefaults)
	buildParameters, err := resolver.Resolve(inv, args.Action, templateVersionParameters)
	if err != nil {
		return nil, err
	}

	err = cliui.ExternalAuth(ctx, inv.Stdout, cliui.ExternalAuthOptions{
		Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionExternalAuth, error) {
			return client.TemplateVersionExternalAuth(ctx, templateVersion.ID)
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("template version git auth: %w", err)
	}

	// Run a dry-run with the given parameters to check correctness
	dryRun, err := client.CreateTemplateVersionDryRun(inv.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
		WorkspaceName:       args.NewWorkspaceName,
		RichParameterValues: buildParameters,
	})
	if err != nil {
		return nil, xerrors.Errorf("begin workspace dry-run: %w", err)
	}
	_, _ = fmt.Fprintln(inv.Stdout, "Planning workspace...")
	err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			return client.TemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
		},
		Cancel: func() error {
			return client.CancelTemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRun.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionDryRunLogsAfter(inv.Context(), templateVersion.ID, dryRun.ID, 0)
		},
		// Don't show log output for the dry-run unless there's an error.
		Silent: true,
	})
	if err != nil {
		// TODO (Dean): reprompt for parameter values if we deem it to
		// be a validation error
		return nil, xerrors.Errorf("dry-run workspace: %w", err)
	}

	resources, err := client.TemplateVersionDryRunResources(inv.Context(), templateVersion.ID, dryRun.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace dry-run resources: %w", err)
	}

	err = cliui.WorkspaceResources(inv.Stdout, resources, cliui.WorkspaceResourcesOptions{
		WorkspaceName: args.NewWorkspaceName,
		// Since agents haven't connected yet, hiding this makes more sense.
		HideAgentState: true,
		Title:          "Workspace Preview",
	})
	if err != nil {
		return nil, xerrors.Errorf("get resources: %w", err)
	}

	return buildParameters, nil
}
