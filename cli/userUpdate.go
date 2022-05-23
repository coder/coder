package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/coder/coder/cli/cliui"
)

func userResetGitSSH() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "regen-ssh <username|user_id|'me'>",
		Short: "Generates a new ssh key for the user. The old ssh key will be deleted.",
		Long: "Generates a new ssh key for the user. The old ssh key will be deleted. " +
			"The command outputs public key of the new ssh key.",
		Args:    cobra.ExactArgs(1),
		Example: "coder users regen-ssh me",
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

			gitSSH, err := client.RegenerateGitSSHKey(ctx, args[0])
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
