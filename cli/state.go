package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func state() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "state",
		Short: "Manually manage Terraform state to fix broken workspaces",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(statePull(), statePush())
	return cmd
}

func statePull() *cobra.Command {
	var buildNumber int
	cmd := &cobra.Command{
		Use:   "pull <workspace> [file]",
		Short: "Pull a Terraform state file from a workspace.",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				workspace, err := namedWorkspace(cmd, client, args[0])
				if err != nil {
					return err
				}
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(cmd.Context(), codersdk.Me, args[0], strconv.Itoa(buildNumber))
				if err != nil {
					return err
				}
			}

			state, err := client.WorkspaceBuildState(cmd.Context(), build.ID)
			if err != nil {
				return err
			}

			if len(args) < 2 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(state))
				return nil
			}

			return os.WriteFile(args[1], state, 0o600)
		},
	}
	cmd.Flags().IntVarP(&buildNumber, "build", "b", 0, "Specify a workspace build to target by name.")
	return cmd
}

func statePush() *cobra.Command {
	var buildNumber int
	cmd := &cobra.Command{
		Use:   "push <workspace> <file>",
		Args:  cobra.ExactArgs(2),
		Short: "Push a Terraform state file to a workspace.",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			if buildNumber == 0 {
				build = workspace.LatestBuild
			} else {
				build, err = client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(cmd.Context(), codersdk.Me, args[0], strconv.Itoa(buildNumber))
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

			build, err = client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: build.TemplateVersionID,
				Transition:        build.Transition,
				ProvisionerState:  state,
			})
			if err != nil {
				return err
			}
			return cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStderr(), client, build.ID)
		},
	}
	cmd.Flags().IntVarP(&buildNumber, "build", "b", 0, "Specify a workspace build to target by name.")
	return cmd
}
