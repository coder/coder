package cli

import (
	"os"
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func gitssh() *cobra.Command {
	return &cobra.Command{
		Use: "gitssh",
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create codersdk client: %w", err)
			}

			key, err := client.GitSSHKey(cmd.Context())
			if err != nil {
				return xerrors.Errorf("get agent git ssh token: %w", err)
			}

			f, err := os.CreateTemp("", "coder-gitsshkey-*")
			if err != nil {
				return xerrors.Errorf("create temp gitsshkey file: %w", err)
			}
			defer func() {
				_ = f.Close()
				_ = os.Remove(f.Name())
			}()
			_, err = f.WriteString(key.PrivateKey)
			if err != nil {
				return xerrors.Errorf("write to temp gitsshkey file: %w", err)
			}
			err = f.Close()
			if err != nil {
				return xerrors.Errorf("close temp gitsshkey file: %w", err)
			}

			a := append([]string{"-i", f.Name()}, args...)
			c := exec.Command("ssh", a...)
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
