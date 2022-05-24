package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/autobuild/schedule"
	"github.com/coder/coder/codersdk"
)

func create() *cobra.Command {
	var (
		autostartMinute string
		autostartHour   string
		autostartDow    string
		parameterFile   string
		templateName    string
		ttl             time.Duration
		tzName          string
		workspaceName   string
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
						_, err = client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, workspaceName)
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

			tz, err := time.LoadLocation(tzName)
			if err != nil {
				return xerrors.Errorf("Invalid workspace autostart timezone: %w", err)
			}
			schedSpec := fmt.Sprintf("CRON_TZ=%s %s %s * * %s", tz.String(), autostartMinute, autostartHour, autostartDow)
			_, err = schedule.Weekly(schedSpec)
			if err != nil {
				return xerrors.Errorf("invalid workspace autostart schedule: %w", err)
			}

			if ttl == 0 {
				return xerrors.Errorf("TTL must be at least 1 minute")
			}

			_, err = client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, workspaceName)
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

			resources, err := client.TemplateVersionResources(cmd.Context(), templateVersion.ID)
			if err != nil {
				return err
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

			before := time.Now()
			workspace, err := client.CreateWorkspace(cmd.Context(), organization.ID, codersdk.CreateWorkspaceRequest{
				TemplateID:        template.ID,
				Name:              workspaceName,
				AutostartSchedule: &schedSpec,
				TTL:               &ttl,
				ParameterValues:   parameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStdout(), client, workspace.LatestBuild.ID, before)
			if err != nil {
				return err
			}

			resources, err = client.WorkspaceResourcesByBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}

			err = cliui.WorkspaceResources(cmd.OutOrStdout(), resources, cliui.WorkspaceResourcesOptions{
				WorkspaceName: workspaceName,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "The %s workspace has been created!\n", cliui.Styles.Keyword.Render(workspace.Name))
			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)
	cliflag.StringVarP(cmd.Flags(), &templateName, "template", "t", "CODER_TEMPLATE_NAME", "", "Specify a template name.")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	cliflag.StringVarP(cmd.Flags(), &autostartMinute, "autostart-minute", "", "CODER_WORKSPACE_AUTOSTART_MINUTE", "0", "Specify the minute(s) at which the workspace should autostart (e.g. 0).")
	cliflag.StringVarP(cmd.Flags(), &autostartHour, "autostart-hour", "", "CODER_WORKSPACE_AUTOSTART_HOUR", "9", "Specify the hour(s) at which the workspace should autostart (e.g. 9).")
	cliflag.StringVarP(cmd.Flags(), &autostartDow, "autostart-day-of-week", "", "CODER_WORKSPACE_AUTOSTART_DOW", "MON-FRI", "Specify the days(s) on which the workspace should autostart (e.g. MON,TUE,WED,THU,FRI)")
	cliflag.StringVarP(cmd.Flags(), &tzName, "tz", "", "TZ", "", "Specify your timezone location for workspace autostart (e.g. US/Central).")
	cliflag.DurationVarP(cmd.Flags(), &ttl, "ttl", "", "CODER_WORKSPACE_TTL", 8*time.Hour, "Specify a time-to-live (TTL) for the workspace (e.g. 8h).")
	return cmd
}
