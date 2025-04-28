package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/coderd/util/ptr"
	"github.com/coder/coder/v2/coderd/util/slice"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) create() *serpent.Command {
	var (
		templateName    string
		templateVersion string
		startAt         string
		stopAfter       time.Duration
		workspaceName   string

		parameterFlags     workspaceParameterFlags
		autoUpdates        string
		copyParametersFrom string
		// Organization context is only required if more than 1 template
		// shares the same name across multiple organizations.
		orgContext = NewOrganizationContext()
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "create [workspace]",
		Short:       "Create a workspace",
		Long: FormatExamples(
			Example{
				Description: "Create a workspace for another user (if you have permission)",
				Command:     "coder create <username>/<workspace_name>",
			},
		),
		Middleware: serpent.Chain(r.InitClient(client)),
		Handler: func(inv *serpent.Invocation) error {
			var err error
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
						err = codersdk.NameValid(workspaceName)
						if err != nil {
							return xerrors.Errorf("workspace name %q is invalid: %w", workspaceName, err)
						}
						_, err = client.WorkspaceByOwnerAndName(inv.Context(), workspaceOwner, workspaceName, codersdk.WorkspaceOptions{})
						if err == nil {
							return xerrors.Errorf("a workspace already exists named %q", workspaceName)
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}
			err = codersdk.NameValid(workspaceName)
			if err != nil {
				return xerrors.Errorf("workspace name %q is invalid: %w", workspaceName, err)
			}
			_, err = client.WorkspaceByOwnerAndName(inv.Context(), workspaceOwner, workspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				return xerrors.Errorf("a workspace already exists named %q", workspaceName)
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
			switch {
			case templateName == "":
				_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Wrap, "Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.Templates(inv.Context(), codersdk.TemplateFilter{})
				if err != nil {
					return err
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
			case sourceWorkspace.LatestBuild.TemplateVersionID != uuid.Nil:
				template, err = client.Template(inv.Context(), sourceWorkspace.TemplateID)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
				templateVersionID = sourceWorkspace.LatestBuild.TemplateVersionID
			default:
				templates, err := client.Templates(inv.Context(), codersdk.TemplateFilter{
					ExactName: templateName,
				})
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
				}
				if len(templates) == 0 {
					return xerrors.Errorf("no template found with the name %q", templateName)
				}

				if len(templates) > 1 {
					templateOrgs := []string{}
					for _, tpl := range templates {
						templateOrgs = append(templateOrgs, tpl.OrganizationName)
					}

					selectedOrg, err := orgContext.Selected(inv, client)
					if err != nil {
						return xerrors.Errorf("multiple templates found with the name %q, use `--org=<organization_name>` to specify which template by that name to use. Organizations available: %s", templateName, strings.Join(templateOrgs, ", "))
					}

					index := slices.IndexFunc(templates, func(i codersdk.Template) bool {
						return i.OrganizationID == selectedOrg.ID
					})
					if index == -1 {
						return xerrors.Errorf("no templates found with the name %q in the organization %q. Templates by that name exist in organizations: %s. Use --org=<organization_name> to select one.", templateName, selectedOrg.Name, strings.Join(templateOrgs, ", "))
					}

					// remake the list with the only template selected
					templates = []codersdk.Template{templates[index]}
				}

				template = templates[0]
				templateVersionID = template.ActiveVersionID
			}

			if len(templateVersion) > 0 {
				version, err := client.TemplateVersionByName(inv.Context(), template.ID, templateVersion)
				if err != nil {
					return xerrors.Errorf("get template version by name: %w", err)
				}
				templateVersionID = version.ID
			}

			// If the user specified an organization via a flag or env var, the template **must**
			// be in that organization. Otherwise, we should throw an error.
			orgValue, orgValueSource := orgContext.ValueSource(inv)
			if orgValue != "" && !(orgValueSource == serpent.ValueSourceDefault || orgValueSource == serpent.ValueSourceNone) {
				selectedOrg, err := orgContext.Selected(inv, client)
				if err != nil {
					return err
				}

				if template.OrganizationID != selectedOrg.ID {
					orgNameFormat := "'--org=%q'"
					if orgValueSource == serpent.ValueSourceEnv {
						orgNameFormat = "CODER_ORGANIZATION=%q"
					}

					return xerrors.Errorf("template is in organization %q, but %s was specified. Use %s to use this template",
						template.OrganizationName,
						fmt.Sprintf(orgNameFormat, selectedOrg.Name),
						fmt.Sprintf(orgNameFormat, template.OrganizationName),
					)
				}
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

			workspace, err := client.CreateUserWorkspace(inv.Context(), workspaceOwner, codersdk.CreateWorkspaceRequest{
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

			cliutil.WarnMatchedProvisioners(inv.Stderr, workspace.LatestBuild.MatchedProvisioners, workspace.LatestBuild.Job)

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
			Flag:        "template-version",
			Env:         "CODER_TEMPLATE_VERSION",
			Description: "Specify a template version name.",
			Value:       serpent.StringOf(&templateVersion),
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
	orgContext.AttachOptions(cmd)
	return cmd
}

type prepWorkspaceBuildArgs struct {
	Action            WorkspaceCLIAction
	TemplateVersionID uuid.UUID
	NewWorkspaceName  string

	LastBuildParameters       []codersdk.WorkspaceBuildParameter
	SourceWorkspaceParameters []codersdk.WorkspaceBuildParameter

	PromptEphemeralParameters bool
	EphemeralParameters       []codersdk.WorkspaceBuildParameter

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
		WithPromptEphemeralParameters(args.PromptEphemeralParameters).
		WithEphemeralParameters(args.EphemeralParameters).
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

	// Attempts to run the dry-run with the given parameters
	// If validation fails, will try to reprompt for the invalid parameters
	var dryRunJob codersdk.ProvisionerJob
	var matchedProvisioners codersdk.MatchedProvisioners
	
	for {
		// Run a dry-run with the given parameters to check correctness
		var err error
		dryRunJob, err = client.CreateTemplateVersionDryRun(inv.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
			WorkspaceName:       args.NewWorkspaceName,
			RichParameterValues: buildParameters,
		})
		if err != nil {
			return nil, xerrors.Errorf("begin workspace dry-run: %w", err)
		}

		matchedProvisioners, err = client.TemplateVersionDryRunMatchedProvisioners(inv.Context(), templateVersion.ID, dryRunJob.ID)
		if err != nil {
			return nil, xerrors.Errorf("get matched provisioners: %w", err)
		}
		cliutil.WarnMatchedProvisioners(inv.Stdout, &matchedProvisioners, dryRunJob)
		_, _ = fmt.Fprintln(inv.Stdout, "Planning workspace...")
		err = cliui.ProvisionerJob(inv.Context(), inv.Stdout, cliui.ProvisionerJobOptions{
			Fetch: func() (codersdk.ProvisionerJob, error) {
				return client.TemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRunJob.ID)
			},
			Cancel: func() error {
				return client.CancelTemplateVersionDryRun(inv.Context(), templateVersion.ID, dryRunJob.ID)
			},
			Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
				return client.TemplateVersionDryRunLogsAfter(inv.Context(), templateVersion.ID, dryRunJob.ID, 0)
			},
			// Don't show log output for the dry-run unless there's an error.
			Silent: true,
		})
		if err == nil {
			// Success, we can continue
			break
		}

		// Check if this is a validation error we can recover from
		var sdkErr *codersdk.Error
		if xerrors.As(err, &sdkErr) && len(sdkErr.Validations) > 0 {
			_, _ = fmt.Fprintf(inv.Stderr, "\n%s\n\n", pretty.Sprint(cliui.DefaultStyles.Error, "Parameter validation failed. Let's fix the invalid values."))
			
			// Track which parameters have been reprompted to avoid duplicates
			repromptedParams := make(map[string]bool)

			// Prompt for each invalid parameter
			for _, validation := range sdkErr.Validations {
				if repromptedParams[validation.Field] {
					continue
				}

				// Find the parameter definition
				var matchingParam *codersdk.TemplateVersionParameter
				for i, param := range templateVersionParameters {
					if param.Name == validation.Field {
						matchingParam = &templateVersionParameters[i]
						break
					}
				}

				if matchingParam != nil {
					_, _ = fmt.Fprintf(inv.Stderr, "Parameter %q: %s\n", validation.Field, validation.Detail)
					parameterValue, promptErr := cliui.RichParameter(inv, *matchingParam, make(map[string]string))
					if promptErr != nil {
						return nil, xerrors.Errorf("prompt for parameter: %w", promptErr)
					}

					// Update the parameter value in buildParameters
					found := false
					for i := range buildParameters {
						if buildParameters[i].Name == validation.Field {
							buildParameters[i].Value = parameterValue
							found = true
							break
						}
					}

					if !found {
						buildParameters = append(buildParameters, codersdk.WorkspaceBuildParameter{
							Name:  validation.Field,
							Value: parameterValue,
						})
					}

					repromptedParams[validation.Field] = true
				}
			}

			// Try the dry run again with updated parameters
			continue
		}

		// Not a validation error or couldn't handle it, so return the original error
		return nil, xerrors.Errorf("dry-run workspace: %w", err)
	}

	resources, err := client.TemplateVersionDryRunResources(inv.Context(), templateVersion.ID, dryRunJob.ID)
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
