package cli

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) start() *serpent.Command {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "start <workspace>",
		Short:       "Start a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{cliui.SkipPromptOption()},
		Handler: func(inv *serpent.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			var build codersdk.WorkspaceBuild
			switch workspace.LatestBuild.Status {
			case codersdk.WorkspaceStatusRunning:
				_, _ = fmt.Fprintf(
					inv.Stdout, "\nThe %s workspace is already running!\n",
					cliui.Keyword(workspace.Name),
				)
				return nil
			case codersdk.WorkspaceStatusStarting:
				_, _ = fmt.Fprintf(
					inv.Stdout, "\nThe %s workspace is already starting.\n",
					cliui.Keyword(workspace.Name),
				)
				build = workspace.LatestBuild
			default:
				build, err = startWorkspace(inv, client, workspace, parameterFlags, WorkspaceStart)
				// It's possible for a workspace build to fail due to the template requiring starting
				// workspaces with the active version.
				if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusForbidden {
					_, _ = fmt.Fprintln(inv.Stdout, "Unable to start the workspace with the template version from the last build. Policy may require you to restart with the current active template version.")
					build, err = startWorkspace(inv, client, workspace, parameterFlags, WorkspaceUpdate)
					if err != nil {
						return xerrors.Errorf("start workspace with active template version: %w", err)
					}
				} else if err != nil {
					return err
				}
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

	cmd.Options = append(cmd.Options, parameterFlags.allOptions()...)

	return cmd
}

func buildWorkspaceStartRequest(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, parameterFlags workspaceParameterFlags, action WorkspaceCLIAction) (codersdk.CreateWorkspaceBuildRequest, error) {
	version := workspace.LatestBuild.TemplateVersionID

	if workspace.AutomaticUpdates == codersdk.AutomaticUpdatesAlways || action == WorkspaceUpdate {
		version = workspace.TemplateActiveVersionID
		if version != workspace.LatestBuild.TemplateVersionID {
			action = WorkspaceUpdate
		}
	}

	lastBuildParameters, err := client.WorkspaceBuildParameters(inv.Context(), workspace.LatestBuild.ID)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, err
	}

	buildOptions, err := asWorkspaceBuildParameters(parameterFlags.buildOptions)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse build options: %w", err)
	}

	cliRichParameters, err := asWorkspaceBuildParameters(parameterFlags.richParameters)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse rich parameters: %w", err)
	}

	cliRichParameterDefaults, err := asWorkspaceBuildParameters(parameterFlags.richParameterDefaults)
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, xerrors.Errorf("unable to parse rich parameter defaults: %w", err)
	}

	buildParameters, err := prepWorkspaceBuild(inv, client, prepWorkspaceBuildArgs{
		Action:              action,
		TemplateVersionID:   version,
		NewWorkspaceName:    workspace.Name,
		LastBuildParameters: lastBuildParameters,

		PromptBuildOptions:    parameterFlags.promptBuildOptions,
		BuildOptions:          buildOptions,
		PromptRichParameters:  parameterFlags.promptRichParameters,
		RichParameters:        cliRichParameters,
		RichParameterFile:     parameterFlags.richParameterFile,
		RichParameterDefaults: cliRichParameterDefaults,
	})
	if err != nil {
		return codersdk.CreateWorkspaceBuildRequest{}, err
	}

	return codersdk.CreateWorkspaceBuildRequest{
		Transition:          codersdk.WorkspaceTransitionStart,
		RichParameterValues: buildParameters,
		TemplateVersionID:   version,
	}, nil
}

func startWorkspace(inv *serpent.Invocation, client *codersdk.Client, workspace codersdk.Workspace, parameterFlags workspaceParameterFlags, action WorkspaceCLIAction) (codersdk.WorkspaceBuild, error) {
	if workspace.DormantAt != nil {
		_, _ = fmt.Fprintln(inv.Stdout, "Activating dormant workspace...")
		err := client.UpdateWorkspaceDormancy(inv.Context(), workspace.ID, codersdk.UpdateWorkspaceDormancy{
			Dormant: false,
		})
		if err != nil {
			return codersdk.WorkspaceBuild{}, xerrors.Errorf("activate workspace: %w", err)
		}
	}
	req, err := buildWorkspaceStartRequest(inv, client, workspace, parameterFlags, action)
	if err != nil {
		return codersdk.WorkspaceBuild{}, err
	}

	build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, req)
	if err != nil {
		return codersdk.WorkspaceBuild{}, xerrors.Errorf("create workspace build: %w", err)
	}

	return build, nil
}
