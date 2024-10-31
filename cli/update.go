package cli

import (
	"fmt"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) update() *serpent.Command {
	var (
		force          bool
		parameterFlags workspaceParameterFlags
		bflags         buildFlags
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
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

			// update command with --force flag is used as an alias for restart.
			// It can be used to update the workspace parameters for an already up-to-date workspace.
			if force {
				_, _ = fmt.Fprintf(inv.Stdout, "Restarting %s to update the workspace parameters.\n", workspace.Name)
				return r.restart().Handler(inv)
			}

			if !workspace.Outdated && len(parameterFlags.richParameters) != 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace is up-to-date. Please add --force to update the workspace parameters.\n")
				return nil
			}

			if !workspace.Outdated && !parameterFlags.promptRichParameters && !parameterFlags.promptEphemeralParameters && len(parameterFlags.ephemeralParameters) == 0 {
				_, _ = fmt.Fprintf(inv.Stdout, "Workspace is up-to-date.\n")
				return nil
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

	cmd.Options = append(cmd.Options, serpent.Option{
		Flag:        "force",
		Description: "Force the update of the workspace even if it is up-to-date.",
		Value:       serpent.BoolOf(&force),
	})
	return cmd
}
