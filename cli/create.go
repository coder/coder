package cli

import (
	"fmt"
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
		parameterFile string
		templateName  string
		startAt       string
		stopAfter     time.Duration
		workspaceName string
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "create [name]",
		Short:       "Create a workspace from a template",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			organization, err := currentOrganization(cmd, client)
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
					return a.WorkspaceOwnerCount > b.WorkspaceOwnerCount
				})

				templateNames := make([]string, 0, len(templates))
				templateByName := make(map[string]codersdk.Template, len(templates))

				for _, template := range templates {
					templateName := template.Name

					if template.WorkspaceOwnerCount > 0 {
						developerText := "developer"
						if template.WorkspaceOwnerCount != 1 {
							developerText = "developers"
						}

						templateName += cliui.Styles.Placeholder.Render(fmt.Sprintf(" (used by %d %s)", template.WorkspaceOwnerCount, developerText))
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

			templateVersion, err := client.TemplateVersion(cmd.Context(), template.ActiveVersionID)
			if err != nil {
				return err
			}
			parameterSchemas, err := client.TemplateVersionSchema(cmd.Context(), templateVersion.ID)
			if err != nil {
				return err
			}

			// parameterMapFromFile can be nil if parameter file is not specified
			var parameterMapFromFile map[string]string
			if parameterFile != "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("Attempting to read the variables from the parameter file.")+"\r\n")
				parameterMapFromFile, err = createParameterMapFromFile(parameterFile)
				if err != nil {
					return err
				}
			}

			disclaimerPrinted := false
			parameters := make([]codersdk.CreateParameterRequest, 0)
			for _, parameterSchema := range parameterSchemas {
				if !parameterSchema.AllowOverrideSource {
					continue
				}
				if !disclaimerPrinted {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
					disclaimerPrinted = true
				}
				parameterValue, err := getParameterValueFromMapOrInput(cmd, parameterMapFromFile, parameterSchema)
				if err != nil {
					return err
				}
				parameters = append(parameters, codersdk.CreateParameterRequest{
					Name:              parameterSchema.Name,
					SourceValue:       parameterValue,
					SourceScheme:      codersdk.ParameterSourceSchemeData,
					DestinationScheme: parameterSchema.DefaultDestinationScheme,
				})
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout())

			// Run a dry-run with the given parameters to check correctness
			after := time.Now()
			dryRun, err := client.CreateTemplateVersionDryRun(cmd.Context(), templateVersion.ID, codersdk.CreateTemplateVersionDryRunRequest{
				WorkspaceName:   workspaceName,
				ParameterValues: parameters,
			})
			if err != nil {
				return xerrors.Errorf("begin workspace dry-run: %w", err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Planning workspace...")
			err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					return client.TemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
				},
				Cancel: func() error {
					return client.CancelTemplateVersionDryRun(cmd.Context(), templateVersion.ID, dryRun.ID)
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
					return client.TemplateVersionDryRunLogsAfter(cmd.Context(), templateVersion.ID, dryRun.ID, after)
				},
				// Don't show log output for the dry-run unless there's an error.
				Silent: true,
			})
			if err != nil {
				// TODO (Dean): reprompt for parameter values if we deem it to
				// be a validation error
				return xerrors.Errorf("dry-run workspace: %w", err)
			}

			resources, err := client.TemplateVersionDryRunResources(cmd.Context(), templateVersion.ID, dryRun.ID)
			if err != nil {
				return xerrors.Errorf("get workspace dry-run resources: %w", err)
			}

			err = cliui.WorkspaceResources(cmd.OutOrStdout(), resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspaceName,
				// Since agent's haven't connected yet, hiding this makes more sense.
				HideAgentState: true,
				Title:          "Workspace Preview",
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

			workspace, err := client.CreateWorkspace(cmd.Context(), organization.ID, codersdk.CreateWorkspaceRequest{
				TemplateID:        template.ID,
				Name:              workspaceName,
				AutostartSchedule: schedSpec,
				TTLMillis:         ptr.Ref(stopAfter.Milliseconds()),
				ParameterValues:   parameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStdout(), client, workspace.LatestBuild.ID, after)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace has been created!\n", cliui.Styles.Keyword.Render(workspace.Name))
			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)
	cliflag.StringVarP(cmd.Flags(), &templateName, "template", "t", "CODER_TEMPLATE_NAME", "", "Specify a template name.")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	cliflag.StringVarP(cmd.Flags(), &startAt, "start-at", "", "CODER_WORKSPACE_START_AT", "", "Specify the workspace autostart schedule. Check `coder schedule start --help` for the syntax.")
	cliflag.DurationVarP(cmd.Flags(), &stopAfter, "stop-after", "", "CODER_WORKSPACE_STOP_AFTER", 8*time.Hour, "Specify a duration after which the workspace should shut down (e.g. 8h).")
	return cmd
}
