package cli

import (
	"fmt"

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

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n",
				cliui.Styles.Wrap.Render("WARNING: A rename can result in data loss if a resource references the workspace name in the template (e.g volumes). Please backup any data before proceeding."),
			)
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "See: %s\n\n", "https://coder.com/docs/coder-oss/latest/templates/resource-persistence#%EF%B8%8F-persistence-pitfalls")
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text: fmt.Sprintf("Type %q to confirm rename:", workspace.Name),
				Validate: func(s string) error {
					if s == workspace.Name {
						return nil
					}
					return xerrors.Errorf("Input %q does not match %q", s, workspace.Name)
				},
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
