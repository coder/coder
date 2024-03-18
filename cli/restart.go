package cli

import (
	"fmt"
	"net/http"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) restart() *serpent.Command {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "restart <workspace>",
		Short:       "Restart a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{cliui.SkipPromptOption()},
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			out := inv.Stdout

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}

			startReq, err := buildWorkspaceStartRequest(inv, client, workspace, parameterFlags, WorkspaceRestart)
			if err != nil {
				return err
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Restart workspace?",
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

			build, err = client.CreateWorkspaceBuild(ctx, workspace.ID, startReq)
			// It's possible for a workspace build to fail due to the template requiring starting
			// workspaces with the active version.
			if cerr, ok := codersdk.AsError(err); ok && cerr.StatusCode() == http.StatusForbidden {
				_, _ = fmt.Fprintln(inv.Stdout, "Unable to restart the workspace with the template version from the last build. Policy may require you to restart with the current active template version.")
				build, err = startWorkspace(inv, client, workspace, parameterFlags, WorkspaceUpdate)
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

	cmd.Options = append(cmd.Options, parameterFlags.allOptions()...)

	return cmd
}
