package cli

import (
	"errors"
	"fmt"
	"time"

	"github.com/fatih/color"
	"github.com/google/uuid"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/database"
)

func workspaceCreate() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create <project> [name]",
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

			var name string
			if len(args) >= 2 {
				name = args[1]
			} else {
				name, err = prompt(cmd, &promptui.Prompt{
					Label: "What's your workspace's name?",
					Validate: func(s string) error {
						if s == "" {
							return xerrors.Errorf("You must provide a name!")
						}
						workspace, _ := client.Workspace(cmd.Context(), "", s)
						if workspace.ID.String() != uuid.Nil.String() {
							return xerrors.New("A workspace already exists with that name!")
						}
						return nil
					},
				})
				if err != nil {
					if errors.Is(err, promptui.ErrAbort) {
						return nil
					}
					return err
				}
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Previewing project create...\n", color.HiBlackString(">"))

			project, err := client.Project(cmd.Context(), organization.Name, args[0])
			if err != nil {
				return err
			}
			projectVersion, err := client.ProjectVersion(cmd.Context(), organization.Name, project.Name, project.ActiveVersionID.String())
			if err != nil {
				return err
			}
			parameterSchemas, err := client.ProjectImportJobSchemas(cmd.Context(), organization.Name, projectVersion.ImportJobID)
			if err != nil {
				return err
			}
			parameterValues, err := client.ProjectImportJobParameters(cmd.Context(), organization.Name, projectVersion.ImportJobID)
			if err != nil {
				return err
			}
			resources, err := client.ProjectImportJobResources(cmd.Context(), organization.Name, projectVersion.ImportJobID)
			if err != nil {
				return err
			}
			err = displayProjectImportInfo(cmd, parameterSchemas, parameterValues, resources)
			if err != nil {
				return err
			}

			_, err = prompt(cmd, &promptui.Prompt{
				Label:     fmt.Sprintf("Create workspace %s?", color.HiCyanString(name)),
				Default:   "y",
				IsConfirm: true,
			})
			if err != nil {
				if errors.Is(err, promptui.ErrAbort) {
					return nil
				}
				return err
			}

			workspace, err := client.CreateWorkspace(cmd.Context(), "", coderd.CreateWorkspaceRequest{
				ProjectID: project.ID,
				Name:      name,
			})
			if err != nil {
				return err
			}
			history, err := client.CreateWorkspaceHistory(cmd.Context(), "", workspace.Name, coderd.CreateWorkspaceHistoryRequest{
				ProjectVersionID: projectVersion.ID,
				Transition:       database.WorkspaceTransitionStart,
			})
			if err != nil {
				return err
			}

			logs, err := client.WorkspaceProvisionJobLogsAfter(cmd.Context(), organization.Name, history.ProvisionJobID, time.Time{})
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Printf("Terraform: %s\n", log.Output)
			}

			// This command is WIP, and output will change!

			_, _ = fmt.Printf("Created workspace! %s\n", name)
			return nil
		},
	}

	return cmd
}
