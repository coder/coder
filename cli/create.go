package cli

import (
	"context"
	"fmt"
	"io"
	"time"

	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) create() *clibase.Cmd {
	var (
		parameterFile     string
		richParameterFile string
		templateName      string
		startAt           string
		stopAfter         time.Duration
		workspaceName     string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "create [name]",
		Short:       "Create a workspace",
		Middleware:  clibase.Chain(r.InitClient(client)),
		Handler: func(inv *clibase.Invocation) error {
			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}

			if len(inv.Args) >= 1 {
				workspaceName = inv.Args[0]
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

			_, err = client.WorkspaceByOwnerAndName(inv.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			var template codersdk.Template
			if templateName == "" {
				_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Wrap.Render("Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.TemplatesByOrganization(inv.Context(), organization.ID)
				if err != nil {
					return err
				}

				slices.SortFunc(templates, func(a, b codersdk.Template) bool {
					return a.ActiveUserCount > b.ActiveUserCount
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				for _, template := range templates {
					templateName := template.Name

					if template.ActiveUserCount > 0 {
						templateName += cliui.Styles.Placeholder.Render(
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
			} else {
				template, err = client.TemplateByName(inv.Context(), organization.ID, templateName)
				if err != nil {
					return xerrors.Errorf("get template by name: %w", err)
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

			buildParams, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
				Template:          template,
				ExistingParams:    []codersdk.Parameter{},
				ParameterFile:     parameterFile,
				RichParameterFile: richParameterFile,
				NewWorkspaceName:  workspaceName,
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
			} else if template.MaxTTLMillis > 0 {
				ttlMillis = &template.MaxTTLMillis
			}

			workspace, err := client.CreateWorkspace(inv.Context(), organization.ID, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          template.ID,
				Name:                workspaceName,
				AutostartSchedule:   schedSpec,
				TTLMillis:           ttlMillis,
				ParameterValues:     buildParams.parameters,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return xerrors.Errorf("create workspace: %w", err)
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, workspace.LatestBuild.ID)
			if err != nil {
				return xerrors.Errorf("watch build: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, "\nThe %s workspace has been created at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	cmd.Options = append(cmd.Options,
		clibase.Option{
			Flag:          "template",
			FlagShorthand: "t",
			Env:           "CODER_TEMPLATE_NAME",
			Description:   "Specify a template name.",
			Value:         clibase.StringOf(&templateName),
		},
		clibase.Option{
			Flag:        "parameter-file",
			Env:         "CODER_PARAMETER_FILE",
			Description: "Specify a file path with parameter values.",
			Value:       clibase.StringOf(&parameterFile),
		},
		clibase.Option{
			Flag:        "rich-parameter-file",
			Env:         "CODER_RICH_PARAMETER_FILE",
			Description: "Specify a file path with values for rich parameters defined in the template.",
			Value:       clibase.StringOf(&richParameterFile),
		},
		clibase.Option{
			Flag:        "start-at",
			Env:         "CODER_WORKSPACE_START_AT",
			Description: "Specify the workspace autostart schedule. Check coder schedule start --help for the syntax.",
			Value:       clibase.StringOf(&startAt),
		},
		clibase.Option{
			Flag:        "stop-after",
			Env:         "CODER_WORKSPACE_STOP_AFTER",
			Description: "Specify a duration after which the workspace should shut down (e.g. 8h).",
			Value:       clibase.DurationOf(&stopAfter),
		},
		cliui.SkipPromptOption(),
	)

	return cmd
}

type prepWorkspaceBuildArgs struct {
	Template           codersdk.Template
	ExistingParams     []codersdk.Parameter
	ParameterFile      string
	ExistingRichParams []codersdk.WorkspaceBuildParameter
	RichParameterFile  string
	NewWorkspaceName   string

	UpdateWorkspace bool
}

type buildParameters struct {
	// Parameters contains legacy parameters stored in /parameters.
	parameters []codersdk.CreateParameterRequest
	// Rich parameters stores values for build parameters annotated with description, icon, type, etc.
	richParameters []codersdk.WorkspaceBuildParameter
}

// prepWorkspaceBuild will ensure a workspace build will succeed on the latest template version.
// Any missing params will be prompted to the user. It supports legacy and rich parameters.
func prepWorkspaceBuild(inv *clibase.Invocation, client *codersdk.Client, args prepWorkspaceBuildArgs) (*buildParameters, error) {
	ctx := inv.Context()

	var useRichParameters bool
	if len(args.ExistingRichParams) > 0 && len(args.RichParameterFile) > 0 {
		useRichParameters = true
	}

	var useLegacyParameters bool
	if len(args.ExistingParams) > 0 || len(args.ParameterFile) > 0 {
		useLegacyParameters = true
	}

	if useRichParameters && useLegacyParameters {
		return nil, xerrors.Errorf("Rich parameters can't be used together with legacy parameters.")
	}

	templateVersion, err := client.TemplateVersion(ctx, args.Template.ActiveVersionID)
	if err != nil {
		return nil, err
	}

	// Legacy parameters
	parameterSchemas, err := client.TemplateVersionSchema(ctx, templateVersion.ID)
	if err != nil {
		return nil, err
	}

	// parameterMapFromFile can be nil if parameter file is not specified
	var parameterMapFromFile map[string]string
	useParamFile := false
	if args.ParameterFile != "" {
		useParamFile = true
		_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
		parameterMapFromFile, err = createParameterMapFromFile(args.ParameterFile)
		if err != nil {
			return nil, err
		}
	}
	disclaimerPrinted := false
	legacyParameters := make([]codersdk.CreateParameterRequest, 0)
PromptParamLoop:
	for _, parameterSchema := range parameterSchemas {
		if !parameterSchema.AllowOverrideSource {
			continue
		}
		if !disclaimerPrinted {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
			disclaimerPrinted = true
		}

		// Param file is all or nothing
		if !useParamFile {
			for _, e := range args.ExistingParams {
				if e.Name == parameterSchema.Name {
					// If the param already exists, we do not need to prompt it again.
					// The workspace scope will reuse params for each build.
					continue PromptParamLoop
				}
			}
		}

		parameterValue, err := getParameterValueFromMapOrInput(inv, parameterMapFromFile, parameterSchema)
		if err != nil {
			return nil, err
		}

		legacyParameters = append(legacyParameters, codersdk.CreateParameterRequest{
			Name:              parameterSchema.Name,
			SourceValue:       parameterValue,
			SourceScheme:      codersdk.ParameterSourceSchemeData,
			DestinationScheme: parameterSchema.DefaultDestinationScheme,
		})
	}

	if disclaimerPrinted {
		_, _ = fmt.Fprintln(inv.Stdout)
	}

	// Rich parameters
	templateVersionParameters, err := client.TemplateVersionRichParameters(inv.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	parameterMapFromFile = map[string]string{}
	useParamFile = false
	if args.RichParameterFile != "" {
		useParamFile = true
		_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("Attempting to read the variables from the rich parameter file.")+"\r\n")
		parameterMapFromFile, err = createParameterMapFromFile(args.RichParameterFile)
		if err != nil {
			return nil, err
		}
	}
	disclaimerPrinted = false
	richParameters := make([]codersdk.WorkspaceBuildParameter, 0)
PromptRichParamLoop:
	for _, templateVersionParameter := range templateVersionParameters {
		if !disclaimerPrinted {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
			disclaimerPrinted = true
		}

		// Param file is all or nothing
		if !useParamFile {
			for _, e := range args.ExistingRichParams {
				if e.Name == templateVersionParameter.Name {
					// If the param already exists, we do not need to prompt it again.
					// The workspace scope will reuse params for each build.
					continue PromptRichParamLoop
				}
			}
		}

		if args.UpdateWorkspace && !templateVersionParameter.Mutable {
			_, _ = fmt.Fprintln(inv.Stdout, cliui.Styles.Warn.Render(fmt.Sprintf(`Parameter %q is not mutable, so can't be customized after workspace creation.`, templateVersionParameter.Name)))
			continue
		}

		parameterValue, err := getWorkspaceBuildParameterValueFromMapOrInput(inv, parameterMapFromFile, templateVersionParameter)
		if err != nil {
			return nil, err
		}

		richParameters = append(richParameters, *parameterValue)
	}

	if disclaimerPrinted {
		_, _ = fmt.Fprintln(inv.Stdout)
	}

	err = cliui.GitAuth(ctx, inv.Stdout, cliui.GitAuthOptions{
		Fetch: func(ctx context.Context) ([]codersdk.TemplateVersionGitAuth, error) {
			return client.TemplateVersionGitAuth(ctx, templateVersion.ID)
		},
	})
	if err != nil {
		return nil, xerrors.Errorf("template version git auth: %w", err)
	}

	// Run a dry-run with the given parameters to check correctness
	dryRun, err := client.CreateTemplateVersionDryRun(inv.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
		WorkspaceName:       args.NewWorkspaceName,
		ParameterValues:     legacyParameters,
		RichParameterValues: richParameters,
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

	return &buildParameters{
		parameters:     legacyParameters,
		richParameters: richParameters,
	}, nil
}
