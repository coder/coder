package cli

import (
	"net/url"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

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

			a := append([]string{"-i", privateKeyFile.Name()}, args...)
			c := exec.CommandContext(cmd.Context(), "ssh", a...)
			c.Stdout = cmd.OutOrStdout()
			c.Stdin = cmd.InOrStdin()
			err = c.Run()
			if err != nil {
				return xerrors.Errorf("run ssh command: %w", err)
			}

			return nil
		},
	}
}
