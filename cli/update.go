package cli

import (
	"fmt"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/codersdk"
)

func update() *clibase.Command {
	var (
		parameterFile     string
		richParameterFile string
		alwaysPrompt      bool
	)

	cmd := &clibase.Command{
		Annotations: workspaceCommand,
		Use:         "update <workspace>",
		Middleware:  clibase.RequireNArgs(1),
		Short:       "Will update and start a given workspace if it is out of date.",
		Long: "Will update and start a given workspace if it is out of date. Use --always-prompt to change " +
			"the parameter values of the workspace.",
		Handler: func(inv *clibase.Invokation) error {
			client, err := useClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, inv.Args[0])
			if err != nil {
				return err
			}
			if !workspace.Outdated && !alwaysPrompt {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace isn't outdated!\n")
				return nil
			}
			template, err := client.Template(inv.Context(), workspace.TemplateID)
			if err != nil {
				return nil
			}

			var existingParams []codersdk.Parameter
			var existingRichParams []codersdk.WorkspaceBuildParameter
			if !alwaysPrompt {
				existingParams, err = client.Parameters(inv.Context(), codersdk.ParameterWorkspace, workspace.ID)
				if err != nil {
					return nil
				}

				existingRichParams, err = client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
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

			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID:   template.ActiveVersionID,
				Transition:          codersdk.WorkspaceTransitionStart,
				ParameterValues:     buildParams.parameters,
				RichParameterValues: buildParams.richParameters,
			})
			if err != nil {
				return err
			}
			logs, closer, err := client.WorkspaceBuildLogsAfter(inv.Context(), build.ID, 0)
			if err != nil {
				return err
			}
			defer closer.Close()
			for {
				log, ok := <-logs
				if !ok {
					break
				}
				_, _ = fmt.Fprintf(inv.Stdout, "Output: %s\n", log.Output)
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&alwaysPrompt, "always-prompt", false, "Always prompt all parameters. Does not pull parameter values from existing workspace")
	cliflag.StringVarP(cmd.Flags(), &parameterFile, "parameter-file", "", "CODER_PARAMETER_FILE", "", "Specify a file path with parameter values.")
	cliflag.StringVarP(cmd.Flags(), &richParameterFile, "rich-parameter-file", "", "CODER_RICH_PARAMETER_FILE", "", "Specify a file path with values for rich parameters defined in the template.")
	return cmd
}
