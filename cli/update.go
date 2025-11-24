package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) update() *serpent.Command {
	var (
		parameterFlags workspaceParameterFlags
		bflags         buildFlags
	)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "update <workspace>",
		Short:       "Will update and start a given workspace if it is out of date. If the workspace is already running, it will be stopped first.",
		Long:        "Use --always-prompt to change the parameter values of the workspace.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			if !workspace.Outdated && !parameterFlags.promptRichParameters && !parameterFlags.promptEphemeralParameters && len(parameterFlags.ephemeralParameters) == 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace is up-to-date.\n")
				return nil
			}

			// #17840: If the workspace is already running, we will stop it before
			// updating. Simply performing a new start transition may not work if the
			// template specifies ignore_changes.
			if workspace.LatestBuild.Transition == codersdk.WorkspaceTransitionStart {
				build, err := stopWorkspace(inv, client, workspace, bflags)
				if err != nil {
					return xerrors.Errorf("stop workspace: %w", err)
				}
				// Wait for the stop to complete.
				if err := cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID); err != nil {
					return xerrors.Errorf("wait for stop: %w", err)
				}
			}

			build, err := startWorkspace(inv, client, workspace, parameterFlags, bflags, WorkspaceUpdate)
			if err != nil {
				return xerrors.Errorf("start workspace: %w", err)
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

	cmd.Options = append(cmd.Options, parameterFlags.allOptions()...)
	cmd.Options = append(cmd.Options, bflags.cliOptions()...)
	return cmd
}
