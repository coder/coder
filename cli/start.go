package cli

import (
	"fmt"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) start() *clibase.Cmd {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "start <workspace>",
		Short:       "Start a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: append(parameterFlags.cliBuildOptions(), cliui.SkipPromptOption()),
		Handler: func(inv *clibase.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			lastBuildParameters, err := client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}

			template, err := client.Template(inv.Context(), workspace.TemplateID)
			if err != nil {
				return err
			}

			buildOptions, err := asWorkspaceBuildParameters(parameterFlags.buildOptions)
			if err != nil {
				return xerrors.Errorf("unable to parse build options: %w", err)
			}

			buildParameters, err := prepStartWorkspace(inv, client, prepStartWorkspaceArgs{
				Action:   WorkspaceStart,
				Template: template,

				LastBuildParameters: lastBuildParameters,

				PromptBuildOptions: parameterFlags.promptBuildOptions,
				BuildOptions:       buildOptions,
			})
			if err != nil {
				return err
			}

			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition:          codersdk.WorkspaceTransitionStart,
				RichParameterValues: buildParameters,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				inv.Stdout, "\nThe %s workspace has been started at %s!\n",
				cliui.Keyword(workspace.Name), cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	return cmd
}

type prepStartWorkspaceArgs struct {
	Action   WorkspaceCLIAction
	Template codersdk.Template

	LastBuildParameters []codersdk.WorkspaceBuildParameter

	PromptBuildOptions bool
	BuildOptions       []codersdk.WorkspaceBuildParameter
}

func prepStartWorkspace(inv *clibase.Invocation, client *codersdk.Client, args prepStartWorkspaceArgs) ([]codersdk.WorkspaceBuildParameter, error) {
	ctx := inv.Context()

	templateVersion, err := client.TemplateVersion(ctx, args.Template.ActiveVersionID)
	if err != nil {
		return nil, xerrors.Errorf("get template version: %w", err)
	}

	templateVersionParameters, err := client.TemplateVersionRichParameters(inv.Context(), templateVersion.ID)
	if err != nil {
		return nil, xerrors.Errorf("get template version rich parameters: %w", err)
	}

	resolver := new(ParameterResolver).
		WithLastBuildParameters(args.LastBuildParameters).
		WithPromptBuildOptions(args.PromptBuildOptions).
		WithBuildOptions(args.BuildOptions)
	return resolver.Resolve(inv, args.Action, templateVersionParameters)
}
