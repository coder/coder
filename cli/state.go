package cli

import (
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func state() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Manually manage Terraform state to fix broken workspaces",
	}
	cmd.AddCommand(statePull(), statePush())
	return cmd
}

func statePull() *cobra.Command {
	var buildName string
	cmd := &cobra.Command{
		Use:  "pull <workspace> [file]",
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildName == "latest" {
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByName(cmd.Context(), workspace.ID, buildName)
				if err != nil {
					return err
				}
			}

			state, err := client.WorkspaceBuildState(cmd.Context(), build.ID)
			if err != nil {
				return err
			}

			if len(args) < 2 {
				cmd.Println(string(state))
				return nil
			}

			return os.WriteFile(args[1], state, 0600)
		},
	}
	cmd.Flags().StringVarP(&buildName, "build", "b", "latest", "Specify a workspace build to target by name.")
	return cmd
}

func statePush() *cobra.Command {
	var buildName string
	cmd := &cobra.Command{
		Use:  "push <workspace> <file>",
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}

			workspace, err := client.WorkspaceByOwnerAndName(cmd.Context(), organization.ID, codersdk.Me, args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildName == "latest" {
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByName(cmd.Context(), workspace.ID, buildName)
				if err != nil {
					return err
				}
			}

			var state []byte
			if args[1] == "-" {
				state, err = io.ReadAll(cmd.InOrStdin())
			} else {
				state, err = os.ReadFile(args[1])
			}
			if err != nil {
				return err
			}

			before := time.Now()
			build, err = client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: build.TemplateVersionID,
				Transition:        build.Transition,
				ProvisionerState:  state,
			})
			if err != nil {
				return err
			}
			return cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStderr(), client, build.ID, before)
		},
	}
	cmd.Flags().StringVarP(&buildName, "build", "b", "latest", "Specify a workspace build to target by name.")
	return cmd
}
