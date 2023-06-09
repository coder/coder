package cli

import (
	"fmt"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) update() *clibase.Cmd {
	var (
		richParameterFile string
		alwaysPrompt      bool
	)

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "update <workspace>",
		Short:       "Will update and start a given workspace if it is out of date",
		Long:        "Use --always-prompt to change the parameter values of the workspace.",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
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

			var existingRichParams []codersdk.WorkspaceBuildParameter
			if !alwaysPrompt {
				existingRichParams, err = client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
				if err != nil {
					return nil
				}
			}

			buildParams, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
				Template:           template,
				ExistingRichParams: existingRichParams,
				RichParameterFile:  richParameterFile,
				NewWorkspaceName:   workspace.Name,

				UpdateWorkspace: true,
				WorkspaceID:     workspace.LatestBuild.ID,
			})
			if err != nil {
				return nil
			}

			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				TemplateVersionID:   template.ActiveVersionID,
				Transition:          codersdk.WorkspaceTransitionStart,
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

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "always-prompt",
			Description: "Always prompt all parameters. Does not pull parameter values from existing workspace.",

			Value: clibase.BoolOf(&alwaysPrompt),
		},
		{
			Flag:        "rich-parameter-file",
			Description: "Specify a file path with values for rich parameters defined in the template.",
			Env:         "CODER_RICH_PARAMETER_FILE",
			Value:       clibase.StringOf(&richParameterFile),
		},
	}
	return cmd
}
