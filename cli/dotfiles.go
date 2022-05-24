package cli

import (
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
)

func dotfiles() *cobra.Command {
	var (
		homeDir string
	)
	cmd := &cobra.Command{
		Use:     "dotfiles [git_repo_url]",
		Args:    cobra.ExactArgs(1),
		Short:   "Checkout and install a dotfiles repository.",
		Example: "coder dotfiles [-y] git@github.com:example/dotfiles.git",
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				dotfilesRepoDir = "dotfiles"
				gitRepo         = args[0]
				cfgDir          = string(createConfig(cmd))
				dotfilesDir     = filepath.Join(cfgDir, dotfilesRepoDir)
				subcommands     = []string{"clone", args[0], dotfilesRepoDir}
				gitCmdDir       = cfgDir
				promptText      = fmt.Sprintf("Cloning %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
				// This follows the same pattern outlined by others in the market:
				// https://github.com/coder/coder/pull/1696#issue-1245742312
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
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found dotfiles repository at %s\n", dotfilesDir)
				gitCmdDir = dotfilesDir
				subcommands = []string{"pull", "--ff-only"}
				promptText = fmt.Sprintf("Pulling latest from %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Did not find dotfiles repository at %s\n", dotfilesDir)
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      promptText,
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

			// check if git ssh command already exists so we can just wrap it
			gitsshCmd := os.Getenv("GIT_SSH_COMMAND")
			if gitsshCmd == "" {
				gitsshCmd = "ssh"
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

			var dotfiles []string
			for _, f := range files {
				// make sure we do not copy `.git*` files
				if strings.HasPrefix(f.Name(), ".") && !strings.HasPrefix(f.Name(), ".git") {
					dotfiles = append(dotfiles, f.Name())
				}
			}

			script := findScript(installScriptSet, files)
			if script == "" {
				if len(dotfiles) == 0 {
					_, _ = fmt.Fprintln(cmd.OutOrStdout(), "No install scripts or dotfiles found, nothing to do.")
					return nil
				}

				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      "No install scripts found, symlinking dotfiles to home directory.\n\n  Continue?",
					IsConfirm: true,
				})
				if err != nil {
					return err
				}

				if homeDir == "" {
					homeDir, err = os.UserHomeDir()
					if err != nil {
						return xerrors.Errorf("getting user home: %w", err)
					}
				}

				for _, df := range dotfiles {
					from := filepath.Join(dotfilesDir, df)
					to := filepath.Join(homeDir, df)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Symlinking %s to %s...\n", from, to)
					// if file already exists at destination remove it
					// this behavior matches `ln -f`
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

			// run found install scripts
			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      fmt.Sprintf("Running install script %s.\n\n  Continue?", script),
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Running %s...\n", script)
			// it is safe to use a variable command here because it's from
			// a filtered list of pre-approved install scripts
			// nolint:gosec
			scriptCmd := exec.CommandContext(cmd.Context(), fmt.Sprintf("./%s", script))
			scriptCmd.Dir = dotfilesDir
			out, err = scriptCmd.CombinedOutput()
			if err != nil {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Error.Render(string(out)))
				return xerrors.Errorf("running %s: %w", script, err)
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), string(out))

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dotfiles installation complete.")
			return nil
		},
	}
	cliui.AllowSkipPrompt(cmd)
	cliflag.StringVarP(cmd.Flags(), &homeDir, "home-dir", "-d", "CODER_HOME_DIR", "", "Specifies the home directory for the dotfiles symlink destination. If empty will use $HOME.")

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

func findScript(installScriptSet []string, files []fs.DirEntry) string {
	for _, i := range installScriptSet {
		for _, f := range files {
			if f.Name() == i {
				return f.Name()
			}
		}
	}

	return ""
}
