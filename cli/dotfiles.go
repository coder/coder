package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
)

func dotfiles() *cobra.Command {
	var symlinkDir string
	cmd := &cobra.Command{
		Use:   "dotfiles [git_repo_url]",
		Args:  cobra.ExactArgs(1),
		Short: "Checkout and install a dotfiles repository from a Git URL",
		Example: formatExamples(
			example{
				Description: "Check out and install a dotfiles repository without prompts",
				Command:     "coder dotfiles --yes git@github.com:example/dotfiles.git",
			},
		),
		RunE: func(cmd *cobra.Command, args []string) error {
			var (
				dotfilesRepoDir = "dotfiles"
				gitRepo         = args[0]
				cfg             = createConfig(cmd)
				cfgDir          = string(cfg)
				dotfilesDir     = filepath.Join(cfgDir, dotfilesRepoDir)
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

			moved := false
			if dotfilesExists {
				du, err := cfg.DotfilesURL().Read()
				if err != nil && !errors.Is(err, os.ErrNotExist) {
					return xerrors.Errorf("reading dotfiles url config: %w", err)
				}
				// if the git url has changed we create a backup and clone fresh
				if gitRepo != du {
					backupDir := fmt.Sprintf("%s_backup_%s", dotfilesDir, time.Now().Format(time.RFC3339))
					_, err = cliui.Prompt(cmd, cliui.PromptOptions{
						Text:      fmt.Sprintf("The dotfiles URL has changed from %q to %q.\n  Coder will backup the existing repo to %s.\n\n  Continue?", du, gitRepo, backupDir),
						IsConfirm: true,
					})
					if err != nil {
						return err
					}

					err = os.Rename(dotfilesDir, backupDir)
					if err != nil {
						return xerrors.Errorf("renaming dir %s: %w", dotfilesDir, err)
					}
					_, _ = fmt.Fprint(cmd.OutOrStdout(), "Done backup up dotfiles.\n")
					dotfilesExists = false
					moved = true
				}
			}

			var (
				gitCmdDir   string
				subcommands []string
				promptText  string
			)
			if dotfilesExists {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Found dotfiles repository at %s\n", dotfilesDir)
				gitCmdDir = dotfilesDir
				subcommands = []string{"pull", "--ff-only"}
				promptText = fmt.Sprintf("Pulling latest from %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			} else {
				if !moved {
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Did not find dotfiles repository at %s\n", dotfilesDir)
				}
				gitCmdDir = cfgDir
				subcommands = []string{"clone", args[0], dotfilesRepoDir}
				promptText = fmt.Sprintf("Cloning %s into directory %s.\n\n  Continue?", gitRepo, dotfilesDir)
			}

			_, err = cliui.Prompt(cmd, cliui.PromptOptions{
				Text:      promptText,
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			// ensure command dir exists
			err = os.MkdirAll(gitCmdDir, 0o750)
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
			c.Stdout = cmd.OutOrStdout()
			c.Stderr = cmd.ErrOrStderr()
			err = c.Run()
			if err != nil {
				if !dotfilesExists {
					return err
				}
				// if the repo exists we soft fail the update operation and try to continue
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), cliui.Styles.Error.Render("Failed to update repo, continuing..."))
			}

			// save git repo url so we can detect changes next time
			err = cfg.DotfilesURL().Write(gitRepo)
			if err != nil {
				return xerrors.Errorf("writing dotfiles url config: %w", err)
			}

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
			if script != "" {
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
				scriptCmd := exec.CommandContext(cmd.Context(), filepath.Join(dotfilesDir, script))
				scriptCmd.Dir = dotfilesDir
				scriptCmd.Stdout = cmd.OutOrStdout()
				scriptCmd.Stderr = cmd.ErrOrStderr()
				err = scriptCmd.Run()
				if err != nil {
					return xerrors.Errorf("running %s: %w", script, err)
				}

				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dotfiles installation complete.")
				return nil
			}

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

			if symlinkDir == "" {
				symlinkDir, err = os.UserHomeDir()
				if err != nil {
					return xerrors.Errorf("getting user home: %w", err)
				}
			}

			for _, df := range dotfiles {
				from := filepath.Join(dotfilesDir, df)
				to := filepath.Join(symlinkDir, df)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Symlinking %s to %s...\n", from, to)

				isRegular, err := isRegular(to)
				if err != nil {
					return xerrors.Errorf("checking symlink for %s: %w", to, err)
				}
				// move conflicting non-symlink files to file.ext.bak
				if isRegular {
					backup := fmt.Sprintf("%s.bak", to)
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Moving %s to %s...\n", to, backup)
					err = os.Rename(to, backup)
					if err != nil {
						return xerrors.Errorf("renaming dir %s: %w", to, err)
					}
				}

				err = os.Symlink(from, to)
				if err != nil {
					return xerrors.Errorf("symlinking %s to %s: %w", from, to, err)
				}
			}

			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Dotfiles installation complete.")
			return nil
		},
	}
	cliui.AllowSkipPrompt(cmd)
	cliflag.StringVarP(cmd.Flags(), &symlinkDir, "symlink-dir", "", "CODER_SYMLINK_DIR", "", "Specifies the directory for the dotfiles symlink destinations. If empty will use $HOME.")

	return cmd
}

// dirExists checks if the path exists and is a directory.
func dirExists(name string) (bool, error) {
	fi, err := os.Stat(name)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}

		return false, xerrors.Errorf("stat dir: %w", err)
	}
	if !fi.IsDir() {
		return false, xerrors.New("exists but not a directory")
	}

	return true, nil
}

// findScript will find the first file that matches the script set.
func findScript(scriptSet []string, files []fs.DirEntry) string {
	for _, i := range scriptSet {
		for _, f := range files {
			if f.Name() == i {
				return f.Name()
			}
		}
	}

	return ""
}

// isRegular detects if the file exists and is not a symlink.
func isRegular(to string) (bool, error) {
	fi, err := os.Lstat(to)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, xerrors.Errorf("lstat %s: %w", to, err)
	}

	return fi.Mode().IsRegular(), nil
}
