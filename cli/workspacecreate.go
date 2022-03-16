package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/briandowns/spinner"

	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
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

			project, err := client.ProjectByName(cmd.Context(), organization.ID, projectName)
			if err != nil {
				return err
			}

			workspaceName := args[0]
			_, err = client.WorkspaceByName(cmd.Context(), "", workspaceName)
			if err == nil {
				return xerrors.Errorf("A workspace already exists named %q!", workspaceName)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Previewing project create...\n", caret)

			projectVersion, err := client.ProjectVersion(cmd.Context(), project.ActiveVersionID)
			if err != nil {
				return err
			}
			parameterSchemas, err := client.ProjectVersionSchema(cmd.Context(), projectVersion.ID)
			if err != nil {
				return err
			}
			parameterValues, err := client.ProjectVersionParameters(cmd.Context(), projectVersion.ID)
			if err != nil {
				return err
			}
			resources, err := client.ProjectVersionResources(cmd.Context(), projectVersion.ID)
			if err != nil {
				return err
			}
			err = displayProjectVersionInfo(cmd, parameterSchemas, parameterValues, resources)
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

			workspace, err := client.CreateWorkspace(cmd.Context(), "", codersdk.CreateWorkspaceRequest{
				ProjectID: project.ID,
				Name:      workspaceName,
			})
			if err != nil {
				return err
			}

			spin := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
			spin.Writer = cmd.OutOrStdout()
			spin.Suffix = " Building workspace..."
			err = spin.Color("fgHiGreen")
			if err != nil {
				return err
			}
			spin.Start()
			defer spin.Stop()
			logs, err := client.WorkspaceBuildLogsAfter(cmd.Context(), workspace.LatestBuild.ID, time.Time{})
			if err != nil {
				return err
			}
			logBuffer := make([]codersdk.ProvisionerJobLog, 0, 64)
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				logBuffer = append(logBuffer, log)
			}
			build, err := client.WorkspaceBuild(cmd.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}
			if build.Job.Status != codersdk.ProvisionerJobSucceeded {
				for _, log := range logBuffer {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s %s\n", color.HiGreenString("[tf]"), log.Output)
				}
				return xerrors.New(build.Job.Error)
			}

			_, _ = fmt.Printf("Created workspace! %s\n", workspaceName)
			return nil
		},
	}
	cmd.Flags().StringVarP(&projectName, "project", "p", "", "Specify a project name.")

	return cmd
}
