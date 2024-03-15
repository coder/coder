package cli

import (
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/pretty"
	"github.com/coder/serpent"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) publickey() *serpent.Command {
	var reset bool
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:        "publickey",
		Aliases:    []string{"pubkey"},
		Short:      "Output your Coder public key used for Git operations",
		Middleware: r.InitClient(client),
		Handler: func(inv *serpent.Invocation) error {
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
				"This is your public key for using "+pretty.Sprint(cliui.DefaultStyles.Field, "git")+" in "+
					"Coder. All clones with SSH will be authenticated automatically 🪄.",
			)
			cliui.Infof(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Code, strings.TrimSpace(key.PublicKey))+"\n")
			cliui.Infof(inv.Stdout, "Add to GitHub and GitLab:")
			cliui.Infof(inv.Stdout, "> https://github.com/settings/ssh/new")
			cliui.Infof(inv.Stdout, "> https://gitlab.com/-/profile/keys")

			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "reset",
			Description: "Regenerate your public key. This will require updating the key on any services it's registered with.",
			Value:       serpent.BoolOf(&reset),
		},
		cliui.SkipPromptOption(),
	}

	return cmd
}
