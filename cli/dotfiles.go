package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
)

func dotfiles() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "dotfiles [git_repo_url]",
		Args:  cobra.ExactArgs(1),
		Short: "Checkout and install a dotfiles repository.",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				dotfilesRepoDir  = "dotfiles"
				gitRepo          = args[0]
				cfg              = createConfig(cmd)
				cfgDir           = string(cfg)
				dotfilesDir      = filepath.Join(cfgDir, dotfilesRepoDir)
				subcommands      = []string{"clone", args[0], dotfilesRepoDir}
				gitCmdDir        = cfgDir
				promtText        = fmt.Sprintf("Cloning %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
				installScriptSet = []string{
					"install.sh",
					"install",
					"bootstrap.sh",
					"bootstrap",
					"script/bootstrap",
					"setup.sh",
					"setup",
					"script/setup",
				}
			)

			_, _ = fmt.Fprint(cmd.OutOrStdout(), "Checking if dotfiles repository already exists...\n")
			dotfilesExists, err := dirExists(dotfilesDir)
			if err != nil {
				return xerrors.Errorf("checking dir %s: %w", dotfilesDir, err)
			}

			// if repo exists already do a git pull instead of clone
			if dotfilesExists {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), fmt.Sprintf("Found dotfiles repository at %s\n", dotfilesDir))
				gitCmdDir = dotfilesDir
				subcommands = []string{"pull", "--ff-only"}
				promtText = fmt.Sprintf("Pulling latest from %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), fmt.Sprintf("Did not find dotfiles repository at %s\n", dotfilesDir))
			}

			// check if git ssh command already exists so we can just wrap it
			gitsshCmd := os.Getenv("GIT_SSH_COMMAND")
			if gitsshCmd == "" {
				gitsshCmd = "ssh"
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      promtText,
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			// ensure config dir exists
			err = os.MkdirAll(gitCmdDir, 0750)
			if err != nil {
				return xerrors.Errorf("ensuring dir at %s: %w", gitCmdDir, err)
			}

			// clone or pull repo
			c := exec.CommandContext(cmd.Context(), "git", subcommands...)
			c.Dir = gitCmdDir
			c.Env = append(os.Environ(), fmt.Sprintf(`GIT_SSH_COMMAND=%s -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no`, gitsshCmd))
			out, err := c.CombinedOutput()
			if err != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Error.Render(string(out)))
				return err
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))

			// check for install scripts
			files, err := os.ReadDir(dotfilesDir)
			if err != nil {
				return xerrors.Errorf("reading files in dir %s: %w", dotfilesDir, err)
			}

			var scripts []string
			var dotfiles []string
			for _, f := range files {
				for _, i := range installScriptSet {
					if f.Name() == i {
						scripts = append(scripts, f.Name())
					}
				}

				if strings.HasPrefix(f.Name(), ".") {
					dotfiles = append(dotfiles, f.Name())
				}
			}

			// run found install scripts
			if len(scripts) > 0 {
				t := "Found install script(s). The following script(s) will be executed in order:\n\n"
				for _, s := range scripts {
					t = fmt.Sprintf("%s  - %s\n", t, s)
				}
				t = fmt.Sprintf("%s\n  Continue?", t)
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      t,
					IsConfirm: true,
				})
				if err != nil {
					return err
				}

				for _, s := range scripts {
					_, _ = fmt.Fprint(cmd.OutOrStdout(), fmt.Sprintf("\nRunning %s...\n", s))
					// it is safe to use a variable command here because it's from
					// a filtered list of pre-approved install scripts
					// nolint:gosec
					c := exec.CommandContext(cmd.Context(), fmt.Sprintf("./%s", s))
					c.Dir = dotfilesDir
					out, err := c.CombinedOutput()
					if err != nil {
						_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Error.Render(string(out)))
						return xerrors.Errorf("running %s: %w", s, err)
					}
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))
				}

				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dotfiles installation complete.")
				return nil
			}

			// otherwise symlink dotfiles
			if len(dotfiles) > 0 {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "No install scripts found, symlinking dotfiles to home directory.\n\n  Continue?",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}

				home, err := os.UserHomeDir()
				if err != nil {
					return xerrors.Errorf("getting user home: %w", err)
				}

				for _, df := range dotfiles {
					from := filepath.Join(dotfilesDir, df)
					to := filepath.Join(home, df)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), fmt.Sprintf("Symlinking %s to %s...\n", from, to))
					// if file already exists at destination remove it
					_, err := os.Lstat(to)
					if err == nil {
						err := os.Remove(to)
						if err != nil {
							return xerrors.Errorf("removing destination file %s: %w", to, err)
						}
					}

					err = os.Symlink(from, to)
					if err != nil {
						return xerrors.Errorf("symlinking %s to %s: %w", from, to, err)
					}
				}

				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dotfiles installation complete.")
				return nil
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No install scripts or dotfiles found, nothing to do.")
			return nil
		},
	}
	cliui.AllowSkipPrompt(cmd)

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
