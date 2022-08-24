package cli

import (
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func rename() *cobra.Command {
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "rename <workspace> <new name>",
		Short:       "Rename a workspace",
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "WARNING: A rename can result in loss of home volume if the template references the workspace name. Continue?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			err = client.UpdateWorkspace(cmd.Context(), workspace.ID, codersdk.UpdateWorkspaceRequest{
				Name: args[1],
			})
			if err != nil {
				return xerrors.Errorf("rename workspace: %w", err)
			}
			return nil
		},
	}

	cliui.AllowSkipPrompt(cmd)

	return cmd
}
