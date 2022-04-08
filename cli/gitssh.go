package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func gitssh() *cobra.Command {
	return &cobra.Command{
		Use:    "gitssh",
		Hidden: true,
		Short:  `Wraps the "ssh" command and uses the coder gitssh key for authentication`,
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}
			cfg := createConfig(cmd)
			session, err := cfg.AgentSession().Read()
			if err != nil {
				return xerrors.Errorf("read agent session from config: %w", err)
			}
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
