package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func restart() *cobra.Command {
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "restart <workspace>",
		Short:       "Restart a workspace",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			out := cmd.OutOrStdout()

			_, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Confirm restart workspace?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
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

			_, _ = fmt.Fprintf(out, "\nThe %s workspace has been restarted at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	cliui.AllowSkipPrompt(cmd)
	return cmd
}
