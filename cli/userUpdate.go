package cli

import (
	"fmt"

	"github.com/coder/coder/codersdk"

	"github.com/coder/coder/cli/cliui"

	"github.com/spf13/cobra"
)

func userResetGitSSH() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "regen-ssh",
		Short: "Generates a new ssh key for the logged-in user. The old ssh key will be deleted.",
		Long: "Generates a new ssh key for the logged-in user. The old ssh key will be deleted. " +
			"The command outputs public key of the new ssh key.",
		Example: "coder users regen-ssh",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      "Confirm regenerate a new sshkey for your workspaces?",
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			ctx := cmd.Context()
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			gitSSH, err := client.RegenerateGitSSHKey(ctx, codersdk.Me)
			if err != nil {
				return err
			}

			_, err = fmt.Fprint(cmd.OutOrStdout(), gitSSH.PublicKey)
			return err
		},
	}

	cliui.AllowSkipPrompt(cmd)
	return cmd
}
