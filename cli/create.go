package cli

import (
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/util/ptr"
	"github.com/coder/coder/codersdk"
)

func create() *cobra.Command {
	var (
		parameterFile     string
		richParameterFile string
		templateName      string
		startAt           string
		stopAfter         time.Duration
		workspaceName     string
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "create [name]",
		Short:       "Create a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			organization, err := CurrentOrganization(cmd, client)
			if err != nil {
				return err
			}

			if len(args) >= 1 {
				workspaceName = args[0]
			}

			if workspaceName == "" {
				workspaceName, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text: "Specify a name for your workspace:",
					Validate: func(workspaceName string) error {
						_, err = client.WorkspaceByOwnerAndName(cmd.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
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

			_, err = client.WorkspaceByOwnerAndName(cmd.Context(), codersdk.Me, workspaceName, codersdk.WorkspaceOptions{})
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			var template codersdk.Template
			if templateName == "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Select a template below to preview the provisioned infrastructure:"))

				templates, err := client.TemplatesByOrganization(cmd.Context(), organization.ID)
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
				option, err := cliui.Select(cmd, cliui.SelectOptions{
					Options:    templateNames,
					HideSearch: true,
				})
				if err != nil {
					return err
				}

				template = templateByName[option]
			} else {
				template, err = client.TemplateByName(cmd.Context(), organization.ID, templateName)
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

			buildParams, err := prepWorkspaceBuild(cmd, client, prepWorkspaceBuildArgs{
				Template:          template,
				ExistingParams:    []codersdk.Parameter{},
				ParameterFile:     parameterFile,
				RichParameterFile: richParameterFile,
				NewWorkspaceName:  workspaceName,
			})
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Confirm create?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := client.CreateWorkspace(cmd.Context(), organization.ID, codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:          template.ID,
				Name:                workspaceName,
				AutostartSchedule:   schedSpec,
				TTLMillis:           ptr.Ref(stopAfter.Milliseconds()),
				ParameterValues:     buildParams.parameters,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStdout(), client, workspace.LatestBuild.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace has been created at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)
	cliflag.StringVarP(cmd.Flags(), &templateName, "template", "t", "CODER_TEMPLATE_NAME", "", "Specify a template name.")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	cliflag.StringVarP(cmd.Flags(), &richParameterFile, "rich-parameter-file", "", "CODER_RICH_PARAMETER_FILE", "", "Specify a file path with values for rich parameters defined in the template.")
	cliflag.StringVarP(cmd.Flags(), &startAt, "start-at", "", "CODER_WORKSPACE_START_AT", "", "Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.")
	cliflag.DurationVarP(cmd.Flags(), &stopAfter, "stop-after", "", "CODER_WORKSPACE_STOP_AFTER", 8*time.Hour, "Specify a duration after which the workspace should shut down (e.g. 8h).")
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
func prepWorkspaceBuild(cmd *cobra.Command, client *codersdk.Client, args prepWorkspaceBuildArgs) (*buildParameters, error) {
	ctx := cmd.Context()

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
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
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

		parameterValue, err := getParameterValueFromMapOrInput(cmd, parameterMapFromFile, parameterSchema)
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
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Rich parameters
	templateVersionParameters, err := client.TemplateVersionRichParameters(cmd.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	parameterMapFromFile = map[string]string{}
	useParamFile = false
	if args.RichParameterFile != "" {
		useParamFile = true
		_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Attempting to read the variables from the rich parameter file.")+"\r\n")
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
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
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Warn.Render(fmt.Sprintf(`Parameter %q is not mutable, so can't be customized after workspace creation.`, templateVersionParameter.Name)))
			continue
		}

		parameterValue, err := getWorkspaceBuildParameterValueFromMapOrInput(cmd, parameterMapFromFile, templateVersionParameter)
		if err != nil {
			return nil, err
		}

		richParameters = append(richParameters, *parameterValue)
	}

	if disclaimerPrinted {
		_, _ = fmt.Fprintln(cmd.OutOrStdout())
	}

	// Run a dry-run with the given parameters to check correctness
	dryRun, err := client.CreateTemplateVersionDryRun(cmd.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
		WorkspaceName:       args.NewWorkspaceName,
		ParameterValues:     legacyParameters,
		RichParameterValues: richParameters,
	})
	if err != nil {
		return nil, xerrors.Errorf("begin workspace dry-run: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Planning workspace...")
	err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
		Fetch: func() (codersdk.ProvisionerJob, error) {
			return client.TemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
		},
		Cancel: func() error {
			return client.CancelTemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
		},
		Logs: func() (<-chan codersdk.ProvisionerJobLog, io.Closer, error) {
			return client.TemplateVersionDryRunLogsAfter(cmd.Context(), templateVersion.ID, dryRun.ID, 0)
		},
		// Don't show log output for the dry-run unless there's an error.
		Silent: true,
	})
	if err != nil {
		// TODO (Dean): reprompt for parameter values if we deem it to
		// be a validation error
		return nil, xerrors.Errorf("dry-run workspace: %w", err)
	}

	resources, err := client.TemplateVersionDryRunResources(cmd.Context(), templateVersion.ID, dryRun.ID)
	if err != nil {
		return nil, xerrors.Errorf("get workspace dry-run resources: %w", err)
	}

	err = cliui.WorkspaceResources(cmd.OutOrStdout(), resources, cliui.WorkspaceResourcesOptions{
		WorkspaceName: args.NewWorkspaceName,
		// Since agents haven't connected yet, hiding this makes more sense.
		HideAgentState: true,
		Title:          "Workspace Preview",
	})
	if err != nil {
		return nil, err
	}

	return &buildParameters{
		parameters:     legacyParameters,
		richParameters: richParameters,
	}, nil
}
