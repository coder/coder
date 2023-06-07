package cli

import (
	"fmt"
	"time"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) restart() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Annotations: workspaceCommand,
		Use:         "restart <workspace>",
		Short:       "Restart a workspace",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Options: clibase.OptionSet{
			cliui.SkipPromptOption(),
		},
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			out := inv.Stdout

			_, err := cliui.Prompt(inv, cliui.PromptOptions{
				Text:      "Confirm restart workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			workspace, err := namedWorkspace(inv.Context(), client, inv.Args[0])
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

			build, err = client.CreateWorkspaceBuild(ctx, workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStart,
			})
			if err != nil {
				return err
			}
			err = cliui.WorkspaceBuild(ctx, out, client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(out, "\nThe %s workspace has been restarted at %s!\n", cliui.DefaultStyles.Keyword.Render(workspace.Name), cliui.DefaultStyles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	return cmd
}
