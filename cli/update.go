package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) update() *serpent.Cmd {
	var parameterFlags workspaceParameterFlags

	client := new(codersdk.Client)
	cmd := &serpent.Cmd{
		Annotations: workspaceCommand,
		Use:         "update <workspace>",
		Short:       "Will update and start a given workspace if it is out of date",
		Long:        "Use --always-prompt to change the parameter values of the workspace.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			if !workspace.Outdated && !parameterFlags.promptRichParameters && !parameterFlags.promptBuildOptions && len(parameterFlags.buildOptions) == 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace isn't outdated!\n")
				return nil
			}

			build, err := startWorkspace(inv, client, workspace, parameterFlags, WorkspaceUpdate)
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
	return cmd
}
