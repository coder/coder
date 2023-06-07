package cli

import (
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func (r *RootCmd) publickey() *clibase.Cmd {
	var reset bool
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:        "publickey",
		Aliases:    []string{"pubkey"},
		Short:      "Output your Coder public key used for Git operations",
		Middleware: r.InitClient(client),
		Handler: func(inv *clibase.Invocation) error {
			if reset {
				// Confirm prompt if using --reset. We don't want to accidentally
				// reset our public key.
				_, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Confirm regenerate a new sshkey for your workspaces? This will require updating the key " +
						"on any services it is registered with. This action cannot be reverted.",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}

				// Reset the public key, let the retrieve re-read it.
				_, err = client.RegenerateGitSSHKey(inv.Context(), codersdk.Me)
				if err != nil {
					return err
				}
			}

			key, err := client.GitSSHKey(inv.Context(), codersdk.Me)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			cliui.Infof(inv.Stdout,
				"This is your public key for using "+cliui.DefaultStyles.Field.Render("git")+" in "+
					"Coder. All clones with SSH will be authenticated automatically 🪄.\n\n",
			)
			cliui.Infof(inv.Stdout, cliui.DefaultStyles.Code.Render(strings.TrimSpace(key.PublicKey))+"\n\n")
			cliui.Infof(inv.Stdout, "Add to GitHub and GitLab:"+"\n")
			cliui.Infof(inv.Stdout, cliui.DefaultStyles.Prompt.String()+"https://github.com/settings/ssh/new"+"\n")
			cliui.Infof(inv.Stdout, cliui.DefaultStyles.Prompt.String()+"https://gitlab.com/-/profile/keys"+"\n")

			return nil
		},
	}

	cmd.Options = clibase.OptionSet{
		{
			Flag:        "reset",
			Description: "Regenerate your public key. This will require updating the key on any services it's registered with.",
			Value:       clibase.BoolOf(&reset),
		},
		cliui.SkipPromptOption(),
	}

	return cmd
}
