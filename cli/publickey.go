package cli

import (
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func publickey() *cobra.Command {
	return &cobra.Command{
		Use:     "publickey",
		Aliases: []string{"pubkey"},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			key, err := client.GitSSHKey(cmd.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			cmd.Println(cliui.Styles.Wrap.Render(
				"This is your public key for using " + cliui.Styles.Field.Render("git") + " in " +
					"Coder. All clones with SSH will be authenticated automatically 🪄.",
			))
			cmd.Println()
			cmd.Println(cliui.Styles.Code.Render(strings.TrimSpace(key.PublicKey)))
			cmd.Println()
			cmd.Println("Add to GitHub and GitLab:")
			cmd.Println(cliui.Styles.Prompt.String() + "https://github.com/settings/ssh/new")
			cmd.Println(cliui.Styles.Prompt.String() + "https://gitlab.com/-/profile/keys")

			return nil
		},
	}
}
