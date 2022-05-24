package cli

import (
	"github.com/spf13/cobra"
)

func dotfiles() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dotfiles [git_repo_url]",
		Short: "Checkout and install a dotfiles repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			// checkout git repo
			// do install script if exists
			// or symlink dotfiles if not

			return nil
		},
	}

	return cmd
}
