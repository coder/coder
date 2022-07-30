package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
)

func update() *cobra.Command {
	var (
		parameterFile string
		alwaysPrompt  bool
	)

	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "update",
		Short:       "Update a workspace to the latest template version",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return err
			}
			if !workspace.Outdated && !alwaysPrompt {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Workspace isn't outdated!\n")
				return nil
			}
			template, err := client.Template(cmd.Context(), workspace.TemplateID)
			if err != nil {
				return nil
			}

			var existingParams []codersdk.Parameter
			if !alwaysPrompt {
				existingParams, err = client.Parameters(cmd.Context(), codersdk.ParameterWorkspace, workspace.ID)
				if err != nil {
					return nil
				}
			}

			parameters, err := prepWorkspaceBuild(cmd, client, prepWorkspaceBuildArgs{
				Template:         template,
				ExistingParams:   existingParams,
				ParameterFile:    parameterFile,
				NewWorkspaceName: workspace.Name,
			})
			if err != nil {
				return nil
			}

			before := time.Now()
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID: template.ActiveVersionID,
				Transition:        workspace.LatestBuild.Transition,
				ParameterValues:   parameters,
			})
			if err != nil {
				return err
			}
			logs, err := client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, before)
			if err != nil {
				return err
			}
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Output: %s\n", log.Output)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&alwaysPrompt, "always-prompt", false, "Always prompt all parameters. Does not pull parameter values from existing workspace")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	return cmd
}
