package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) stop() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "stop <workspace>",
		Short:       "Stop a workspace",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: serpent.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *serpent.Invocation) error {
			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm stop workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
			if err != nil {
				return err
			}
			build, err := client.CreateWorkspaceBuild(inv.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(inv.Context(), inv.Stdout, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(
				inv.Stdout,
				"\nThe %s workspace has been stopped at %s!\n", cliui.Keyword(workspace.Name),

				cliui.Timestamp(time.Now()),
			)
			return nil
		},
	}
	return cmd
}
