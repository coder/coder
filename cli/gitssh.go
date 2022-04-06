package cli

import (
	"os/exec"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func gitssh() *cobra.Command {
	return &cobra.Command{
		Use: "gitssh",
		RunE: func(cmd *cobra.Command, args []string) error {

			// fmt.Fprintf(os.Stderr, "%s %s", "ssh", strings.Join(append([]string{"-i", "/home/coder/.ssh/coder"}, args...), " "))
			a := append([]string{"-i", "/home/coder/.ssh/coder"}, args...)
			c := exec.Command("ssh", a...)
			err := c.Run()
			if err != nil {
				return xerrors.Errorf("running ssh command: %w", err)
			}

			return nil
		},
	}
}
