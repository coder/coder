package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func stop() *cobra.Command {
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "stop <workspace>",
		Short:       "Stop a workspace",
		Args:        cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Confirm stop workspace?",
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
			build, err := client.CreateWorkspaceBuild(cmd.Context(), workspace.ID, codersdk.CreateWorkspaceBuildRequest{
				Transition: codersdk.WorkspaceTransitionStop,
			})
			if err != nil {
				return err
			}

			err = cliui.WorkspaceBuild(cmd.Context(), cmd.OutOrStdout(), client, build.ID)
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\nThe %s workspace has been stopped at %s!\n", cliui.Styles.Keyword.Render(workspace.Name), cliui.Styles.DateTimeStamp.Render(time.Now().Format(time.Stamp)))
			return nil
		},
	}
	cliui.AllowSkipPrompt(cmd)
	return cmd
}
