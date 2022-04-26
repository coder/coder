package cli

import (
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

func gitssh() *cobra.Command {
	return &cobra.Command{
		Use:    "gitssh",
		Hidden: true,
		Short:  `Wraps the "ssh" command and uses the coder gitssh key for authentication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg := createConfig(cmd)
			rawURL, err := cfg.URL().Read()
			if err != nil {
				return xerrors.Errorf("read agent url from config: %w", err)
			}
			parsedURL, err := url.Parse(rawURL)
			if err != nil {
				return xerrors.Errorf("parse agent url from config: %w", err)
			}
			session, err := cfg.AgentSession().Read()
			if err != nil {
				return xerrors.Errorf("read agent session from config: %w", err)
			}
			client := codersdk.New(parsedURL)
			client.SessionToken = session

			key, err := client.AgentGitSSHKey(cmd.Context())
			if err != nil {
				return xerrors.Errorf("get agent git ssh token: %w", err)
			}

			privateKeyFile, err := os.CreateTemp("", "coder-gitsshkey-*")
			if err != nil {
				return xerrors.Errorf("create temp gitsshkey file: %w", err)
			}
			defer func() {
				_ = privateKeyFile.Close()
				_ = os.Remove(privateKeyFile.Name())
			}()
			_, err = privateKeyFile.WriteString(key.PrivateKey)
			if err != nil {
				return xerrors.Errorf("write to temp gitsshkey file: %w", err)
			}
			err = privateKeyFile.Close()
			if err != nil {
				return xerrors.Errorf("close temp gitsshkey file: %w", err)
			}

			args = append([]string{"-i", privateKeyFile.Name()}, args...)
			c := exec.CommandContext(cmd.Context(), "ssh", args...)
			c.Stderr = cmd.ErrOrStderr()
			c.Stdout = cmd.OutOrStdout()
			c.Stdin = cmd.InOrStdin()
			err = c.Run()
			if err != nil {
				exitErr := &exec.ExitError{}
				if xerrors.As(err, &exitErr) && exitErr.ExitCode() == 255 {
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(),
						"\n"+cliui.Styles.Wrap.Render("Coder authenticates with "+cliui.Styles.Field.Render("git")+
							" using the public key below. All clones with SSH are authenticated automatically 🪄.")+"\n")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Code.Render(strings.TrimSpace(key.PublicKey))+"\n")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), "Add to GitHub and GitLab:")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Prompt.String()+"https://github.com/settings/ssh/new")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Prompt.String()+"https://gitlab.com/-/profile/keys")
					_, _ = fmt.Fprintln(cmd.ErrOrStderr())
					return err
				}
				return xerrors.Errorf("run ssh command: %w", err)
			}

			return nil
		},
	}
}
