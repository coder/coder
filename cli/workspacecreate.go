package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func workspaceCreate() *cobra.Command {
	var (
		workspaceName string
		templateName  string
	)
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a workspace from a template",
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

			printed := false
			parameters := make([]codersdk.CreateParameterRequest, 0)
			for _, parameterSchema := range parameterSchemas {
				if !parameterSchema.AllowOverrideSource {
					continue
				}
				if !printed {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has customizable parameters. Values can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
					printed = true
				}
				value, err := cliui.ParameterSchema(cmd, parameterSchema)
				if err != nil {
					return err
				}
				parameters = append(parameters, codersdk.CreateParameterRequest{
					Name:              parameterSchema.Name,
					SourceValue:       value,
					SourceScheme:      database.ParameterSourceSchemeData,
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
				TemplateID:      template.ID,
				Name:            workspaceName,
				ParameterValues: parameters,
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

	cliflag.StringVarP(cmd.Flags(), &templateName, "template", "t", "CODER_TEMPLATE_NAME", "", "Specify a template name.")
	return cmd
}
