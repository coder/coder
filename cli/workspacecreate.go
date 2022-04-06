package cli

import (
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func workspaceCreate() *cobra.Command {
	var (
		templateName string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Args:  cobra.ExactArgs(1),
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

			var template codersdk.Template
			if templateName == "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Select a template:"))

				templateNames := []string{}
				templateByName := map[string]codersdk.Template{}
				templates, err := client.TemplatesByOrganization(cmd.Context(), organization.ID)
				if err != nil {
					return err
				}
				for _, template := range templates {
					templateNames = append(templateNames, template.Name)
					templateByName[template.Name] = template
				}
				sort.Slice(templateNames, func(i, j int) bool {
					return templateByName[templateNames[i]].WorkspaceOwnerCount > templateByName[templateNames[j]].WorkspaceOwnerCount
				})
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
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Creating with the "+cliui.Styles.Field.Render(template.Name)+" template...")

			workspaceName := args[0]
			_, err = client.WorkspaceByName(cmd.Context(), codersdk.Me, workspaceName)
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
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
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This template has customizable parameters! These can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
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
			if printed {
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.FocusedPrompt.String()+"Previewing resources...")
				_, _ = fmt.Fprintln(cmd.OutOrStdout())
			}
			resources, err := client.TemplateVersionResources(cmd.Context(), templateVersion.ID)
			if err != nil {
				return err
			}
			err = displayTemplateVersionInfo(cmd, resources)
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Create workspace %s?", color.HiCyanString(workspaceName)),
				Default:   "yes",
				IsConfirm: true,
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			before := time.Now()
			workspace, err := client.CreateWorkspace(cmd.Context(), codersdk.Me, codersdk.CreateWorkspaceRequest{
				TemplateID:      template.ID,
				Name:            workspaceName,
				ParameterValues: parameters,
			})
			if err != nil {
				return err
			}
			err = cliui.ProvisionerJob(cmd.Context(), cmd.OutOrStdout(), cliui.ProvisionerJobOptions{
				Fetch: func() (codersdk.ProvisionerJob, error) {
					build, err := client.WorkspaceBuild(cmd.Context(), workspace.LatestBuild.ID)
					return build.Job, err
				},
				Cancel: func() error {
					return client.CancelWorkspaceBuild(cmd.Context(), workspace.LatestBuild.ID)
				},
				Logs: func() (<-chan codersdk.ProvisionerJobLog, error) {
					return client.WorkspaceBuildLogsAfter(cmd.Context(), workspace.LatestBuild.ID, before)
				},
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace has been created!\n\n", cliui.Styles.Keyword.Render(workspace.Name))
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  "+cliui.Styles.Code.Render("coder ssh "+workspace.Name))
			_, _ = fmt.Fprintln(cmd.OutOrStdout())

			return err
		},
	}
	cmd.Flags().StringVarP(&templateName, "template", "p", "", "Specify a template name.")

	return cmd
}
