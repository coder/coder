package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
)

func update() *cobra.Command {
	var (
		parameterFile     string
		richParameterFile string
		alwaysPrompt      bool
	)

	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "update <workspace>",
		Args:        cobra.ExactArgs(1),
		Short:       "Update a workspace",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
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
			var existingRichParams []codersdk.WorkspaceBuildParameter
			if !alwaysPrompt {
				existingParams, err = client.Parameters(cmd.Context(), codersdk.ParameterWorkspace, workspace.ID)
				if err != nil {
					return nil
				}

				existingRichParams, err = client.WorkspaceBuildParameters(cmd.Context(), workspace.LatestBuild.ID)
				if err != nil {
					return nil
				}
			}

			buildParams, err := prepWorkspaceBuild(cmd, client, prepWorkspaceBuildArgs{
				Template:           template,
				ExistingParams:     existingParams,
				ParameterFile:      parameterFile,
				ExistingRichParams: existingRichParams,
				RichParameterFile:  richParameterFile,
				NewWorkspaceName:   workspace.Name,
				UpdateWorkspace:    true,
			})
			if err != nil {
				return nil
			}

			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID:   template.ActiveVersionID,
				Transition:          workspace.LatestBuild.Transition,
				ParameterValues:     buildParams.parameters,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return err
			}
			logs, closer, err := client.WorkspaceBuildLogsAfter(cmd.Context(), build.ID, 0)
			if err != nil {
				return err
			}
			defer closer.Close()
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
	cliflag.StringVarP(cmd.Flags(), &richParameterFile, "rich-parameter-file", "", "CODER_RICH_PARAMETER_FILE", "", "Specify a file path with values for rich parameters defined in the template.")
	return cmd
}
