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

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

func (r *RootCmd) dotfiles() *clibase.Cmd {
	var symlinkDir string
	cmd := &clibase.Cmd{
		Use:        "dotfiles <git_repo_url>",
		Middleware: clibase.RequireNArgs(1),
		Short:      "Personalize your workspace by applying a canonical dotfiles repository",
		Long: formatExamples(
			example{
				Description: "Check out and install a dotfiles repository without prompts",
				Command:     "coder dotfiles --yes git@github.com:example/dotfiles.git",
			},
		),
		Handler: func(inv *clibase.Invocation) error {
			var (
				dotfilesRepoDir = "dotfiles"
				gitRepo         = inv.Args[0]
				cfg             = r.createConfig()
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

			if cfg == "" {
				return xerrors.Errorf("no config directory")
			}

			_, _ = fmt.Fprint(inv.Stdout, "Checking if dotfiles repository already exists...\n")
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
					_, err = cliui.Prompt(inv, cliui.PromptOptions{
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
					_, _ = fmt.Fprint(inv.Stdout, "Done backup up dotfiles.\n")
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
				_, _ = fmt.Fprintf(inv.Stdout, "Found dotfiles repository at %s\n", dotfilesDir)
				gitCmdDir = dotfilesDir
				subcommands = []string{"pull", "--ff-only"}
				promptText = fmt.Sprintf("Pulling latest from %s into directory %s.\n  Continue?", gitRepo, dotfilesDir)
			} else {
				if !moved {
					_, _ = fmt.Fprintf(inv.Stdout, "Did not find dotfiles repository at %s\n", dotfilesDir)
				}
				gitCmdDir = cfgDir
				subcommands = []string{"clone", inv.Args[0], dotfilesRepoDir}
				promptText = fmt.Sprintf("Cloning %s into directory %s.\n\n  Continue?", gitRepo, dotfilesDir)
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
				Text:      promptText,
				IsConfirm: true,
			})
			if err != nil {
				return err
			}

			// ensure command dir exists
			err = os.MkdirAll(gitCmdDir, 0o750)
			if err != nil {
				return xerrors.Errorf("ensuring dir at %q: %w", gitCmdDir, err)
			}

			// check if git ssh command already exists so we can just wrap it
			gitsshCmd := os.Getenv("GIT_SSH_COMMAND")
			if gitsshCmd == "" {
				gitsshCmd = "ssh"
			}

			// clone or pull repo
			c := exec.CommandContext(inv.Context(), "git", subcommands...)
			c.Dir = gitCmdDir
			c.Env = append(inv.Environ.ToOS(), fmt.Sprintf(`GIT_SSH_COMMAND=%s -o UserKnownHostsFile=/dev/null -o StrictHostKeyChecking=no`, gitsshCmd))
			c.Stdout = inv.Stdout
			c.Stderr = inv.Stderr
			err = c.Run()
			if err != nil {
				if !dotfilesExists {
					return err
				}
				// if the repo exists we soft fail the update operation and try to continue
				_, _ = fmt.Fprintln(inv.Stdout, cliui.DefaultStyles.Error.Render("Failed to update repo, continuing..."))
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
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      fmt.Sprintf("Running install script %s.\n\n  Continue?", script),
					IsConfirm: true,
				})
				if err != nil {
					return err
				}

				_, _ = fmt.Fprintf(inv.Stdout, "Running %s...\n", script)
				// it is safe to use a variable command here because it's from
				// a filtered list of pre-approved install scripts
				// nolint:gosec
				scriptCmd := exec.CommandContext(inv.Context(), filepath.Join(dotfilesDir, script))
				scriptCmd.Dir = dotfilesDir
				scriptCmd.Stdout = inv.Stdout
				scriptCmd.Stderr = inv.Stderr
				err = scriptCmd.Run()
				if err != nil {
					return xerrors.Errorf("running %s: %w", script, err)
				}

				_, _ = fmt.Fprintln(inv.Stdout, "Dotfiles installation complete.")
				return nil
			}

			if len(dotfiles) == 0 {
				_, _ = fmt.Fprintln(inv.Stdout, "No install scripts or dotfiles found, nothing to do.")
				return nil
			}

			_, err = cliui.Prompt(inv, cliui.PromptOptions{
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
				_, _ = fmt.Fprintf(inv.Stdout, "Symlinking %s to %s...\n", from, to)

				isRegular, err := isRegular(to)
				if err != nil {
					return xerrors.Errorf("checking symlink for %s: %w", to, err)
				}
				// move conflicting non-symlink files to file.ext.bak
				if isRegular {
					backup := fmt.Sprintf("%s.bak", to)
					_, _ = fmt.Fprintf(inv.Stdout, "Moving %s to %s...\n", to, backup)
					err = os.Rename(to, backup)
					if err != nil {
						return xerrors.Errorf("renaming dir %s: %w", to, err)
					}
				}

				// attempt to delete the file before creating a new symlink.  This overwrites any existing symlinks
				// which are typically leftover from a previous call to coder dotfiles.  We do this best effort and
				// ignore errors because the symlink may or may not exist.  Any regular files are backed up above.
				_ = os.Remove(to)
				err = os.Symlink(from, to)
				if err != nil {
					return xerrors.Errorf("symlinking %s to %s: %w", from, to, err)
				}
			}

			_, _ = fmt.Fprintln(inv.Stdout, "Dotfiles installation complete.")
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "symlink-dir",
			Env:         "CODER_SYMLINK_DIR",
			Description: "Specifies the directory for the dotfiles symlink destinations. If empty, will use $HOME.",
			Value:       clibase.StringOf(&symlinkDir),
		},
		cliui.SkipPromptOption(),
	}
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
