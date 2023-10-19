package cli

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
)

func (r *RootCmd) restart() *clibase.Cmd {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "restart <workspace>",
		Short:       "Restart a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: append(parameterFlags.cliBuildOptions(), cliui.SkipPromptOption()),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			out := inv.Stdout

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			lastBuildParameters, err := client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
			if err != nil {
				return err
			}

			buildOptions, err := asWorkspaceBuildParameters(parameterFlags.buildOptions)
			if err != nil {
				return xerrors.Errorf("can't parse build options: %w", err)
			}

			buildParameters, err := prepStartWorkspace(inv, client, prepStartWorkspaceArgs{
				Action:            WorkspaceRestart,
				TemplateVersionID: workspace.LatestBuild.TemplateVersionID,

				LastBuildParameters: lastBuildParameters,

				PromptBuildOptions: parameterFlags.promptBuildOptions,
				BuildOptions:       buildOptions,
			})
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm restart workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
			})
			if err != nil {
				return err
			}
			err = cliui.WorkspaceBuild(ctx, out, client, build.ID)
			if err != nil {
				return err
			}

			req := codersdk.CreateWorkspaceBuildRequest{
				Transition:          codersdk.WorkspaceTransitionStart,
				RichParameterValues: buildParameters,
				TemplateVersionID:   workspace.LatestBuild.TemplateVersionID,
			}

			build, err = client.CreateWorkspaceBuild(ctx, workspace.ID, req)
			// It's possible for a workspace build to fail due to the template requiring starting
			// workspaces with the active version.
			if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusUnauthorized {
				build, err = startWorkspaceActiveVersion(inv, client, startWorkspaceActiveVersionArgs{
					BuildOptions:        buildOptions,
					LastBuildParameters: lastBuildParameters,
					PromptBuildOptions:  parameterFlags.promptBuildOptions,
					Workspace:           workspace,
				})
				if err != nil {
					return xerrors.Errorf("start workspace with active template version: %w", err)
				}
			} else if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(ctx, out, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(out,
				"\nThe %s workspace has been restarted at %s!\n",
				pretty.Sprint(cliui.DefaultStyles.Keyword, workspace.Name), cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	return cmd
}
