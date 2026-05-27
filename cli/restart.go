package cli

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

func (r *RootCmd) restart() *serpent.Command {
	var (
		parameterFlags workspaceParameterFlags
		bflags         buildFlags
	)

	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "restart <workspace>",
		Short:       "Restart a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Options: serpent.OptionSet{cliui.SkipPromptOption()},
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			ctx := inv.Context()
			out := inv.Stdout

			workspace, err := client.ResolveWorkspace(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}

			startReq, err := buildWorkspaceStartRequest(inv, client, workspace, parameterFlags, bflags, WorkspaceRestart)
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

			stopParamValues, err := asWorkspaceBuildParameters(parameterFlags.ephemeralParameters)
			if err != nil {
				return xerrors.Errorf("parse ephemeral parameters: %w", err)
			}
			wbr := codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
				// Ephemeral parameters should be passed to both stop and start builds.
				// TODO: maybe these values should be sourced from the previous build?
				//  It has to be manually sourced, as ephemeral parameters do not carry across
				//  builds.
				RichParameterValues: stopParamValues,
				LogLevel:            startReq.LogLevel,
				Reason:              startReq.Reason,
				OnSuccess: &codersdk.CreateWorkspaceBuildOnSuccessRequest{
					Transition:          startReq.Transition,
					RichParameterValues: startReq.RichParameterValues,
				},
			}
			build, err := client.CreateWorkspaceBuild(ctx, workspace.ID, wbr)
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(ctx, out, client, build.ID)
			if err != nil {
				return err
			}

			build, err = waitForWorkspaceBuildByNumber(ctx, client, workspace, build.BuildNumber+1)
			if err != nil {
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
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)

	return cmd
}

func waitForWorkspaceBuildByNumber(
	ctx context.Context,
	client *codersdk.Client,
	workspace codersdk.Workspace,
	buildNumber int32,
) (codersdk.WorkspaceBuild, error) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	buildNumberString := strconv.FormatInt(int64(buildNumber), 10)
	for {
		build, err := client.WorkspaceBuildByUsernameAndWorkspaceNameAndBuildNumber(
			ctx,
			workspace.OwnerName,
			workspace.Name,
			buildNumberString,
		)
		if err == nil {
			return build, nil
		}
		if cerr, ok := codersdk.AsError(err); !ok || cerr.StatusCode() != http.StatusNotFound {
			return codersdk.WorkspaceBuild{}, err
		}

		select {
		case <-ctx.Done():
			return codersdk.WorkspaceBuild{}, ctx.Err()
		case <-ticker.C:
		}
	}
}
