package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/coder/coder/cli/cliui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

const (
	dotfilesRepoDir = "dotfiles"
)

func dotfiles() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dotfiles [git_repo_url]",
		Args:  cobra.ExactArgs(1),
		Short: "Checkout and install a dotfiles repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				gitRepo     = args[0]
				cfg         = createConfig(cmd)
				cfgDir      = string(cfg)
				dotfilesDir = filepath.Join(cfgDir, dotfilesRepoDir)
				subcommands = []string{"clone", args[0], dotfilesRepoDir}
				gitCmdDir   = cfgDir
				promtText   = fmt.Sprintf("Cloning %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			)

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Checking if dotfiles repository already exists...")
			dotfilesExists, err := dirExists(dotfilesDir)
			if err != nil {
				return xerrors.Errorf("checking dir %s: %w", dotfilesDir, err)
			}

			// if repo exists already do a git pull instead of clone
			if dotfilesExists {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf("Found dotfiles repository at %s", dotfilesDir))
				gitCmdDir = dotfilesDir
				subcommands = []string{"pull", "--ff-only"}
				promtText = fmt.Sprintf("Pulling latest from %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			} else {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf("Did not find dotfiles repository at %s", dotfilesDir))
			}

			// check if git ssh command already exists so we can just wrap it
			gitsshCmd := os.Getenv("GIT_SSH_COMMAND")
			if gitsshCmd == "" {
				gitsshCmd = "ssh"
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), fmt.Sprintf("gitssh %s", gitsshCmd))

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      promtText,
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			err = os.MkdirAll(gitCmdDir, 0750)
			if err != nil {
				return xerrors.Errorf("ensuring dir at %s: %w", gitCmdDir, err)
			}

			c := exec.CommandContext(cmd.Context(), "git", subcommands...)
			c.Dir = gitCmdDir
			c.Env = append(os.Environ(), fmt.Sprintf(`GIT_SSH_COMMAND=%s -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no`, gitsshCmd))
			out, err := c.CombinedOutput()
			if err != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Error.Render(string(out)))
				return xerrors.Errorf("running git command: %w", err)
			}
			_, _ = fmt.Fprint(cmd.OutOrStdout(), string(out))

			// do install script if exists
			// or symlink dotfiles if not

			return nil
		},
	}

	return cmd
}

func dirExists(name string) (bool, error) {
	_, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, xerrors.Errorf("stat dir: %w", err)
	}

	return true, nil
}
