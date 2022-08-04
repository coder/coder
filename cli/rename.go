package cli

import (
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

func rename() *cobra.Command {
	return &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "rename <workspace> <new name>",
		Short:       "Rename a workspace",
		Args:        cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			workspace, err := namedWorkspace(cmd, client, args[0])
			if err != nil {
				return xerrors.Errorf("get workspace: %w", err)
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
}
