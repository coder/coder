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
		projectName string
	)
	cmd := &cobra.Command{
		Use:   "create <name>",
		Args:  cobra.ExactArgs(1),
		Short: "Create a workspace from a project",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			var project codersdk.Project
			if projectName == "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Wrap.Render("Select a project:"))

				projectNames := []string{}
				projectByName := map[string]codersdk.Project{}
				projects, err := client.ProjectsByOrganization(cmd.Context(), organization.ID)
				if err != nil {
					return err
				}
				for _, project := range projects {
					projectNames = append(projectNames, project.Name)
					projectByName[project.Name] = project
				}
				sort.Slice(projectNames, func(i, j int) bool {
					return projectByName[projectNames[i]].WorkspaceOwnerCount > projectByName[projectNames[j]].WorkspaceOwnerCount
				})
				// Move the cursor up a single line for nicer display!
				option, err := cliui.Select(cmd, cliui.SelectOptions{
					Options:    projectNames,
					HideSearch: true,
				})
				if err != nil {
					return err
				}
				project = projectByName[option]
			} else {
				project, err = client.ProjectByName(cmd.Context(), organization.ID, projectName)
				if err != nil {
					return xerrors.Errorf("get project by name: %w", err)
				}
				if err != nil {
					return err
				}
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout())
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Prompt.String()+"Creating with the "+cliui.Styles.Field.Render(project.Name)+" project...")

			workspaceName := args[0]
			_, err = client.WorkspaceByName(cmd.Context(), codersdk.Me, workspaceName)
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			projectVersion, err := client.ProjectVersion(cmd.Context(), project.ActiveVersionID)
			if err != nil {
				return err
			}
			parameterSchemas, err := client.ProjectVersionSchema(cmd.Context(), projectVersion.ID)
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
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Paragraph.Render("This project has customizable parameters! These can be changed after create, but may have unintended side effects (like data loss).")+"\r\n")
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
			resources, err := client.ProjectVersionResources(cmd.Context(), projectVersion.ID)
			if err != nil {
				return err
			}
			err = displayProjectVersionInfo(cmd, resources)
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
				ProjectID:       project.ID,
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
	cmd.Flags().StringVarP(&projectName, "project", "p", "", "Specify a project name.")

	return cmd
}
