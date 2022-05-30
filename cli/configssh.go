package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/cli/safeexec"
	"github.com/pkg/diff"
	"github.com/pkg/diff/write"
	"github.com/spf13/cobra"
	"golang.org/x/exp/slices"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

const (
	sshDefaultConfigFileName = "~/.ssh/config"
	// TODO(mafredri): Consider moving to coder config dir, i.e. ~/.config/coderv2/ssh_config?
	sshDefaultCoderConfigFileName = "~/.ssh/coder"
	sshCoderConfigHeader          = `# This file is managed by coder. DO NOT EDIT.
#
# You should not hand-edit this file, all changes will be lost upon workspace
# creation, deletion or when running "coder config-ssh".
`
	sshCoderConfigOptionsHeader = `#
# Last config-ssh options:
`
	// Relative paths are assumed to be in ~/.ssh, except when
	// included in /etc/ssh.
	sshConfigIncludeStatement = "Include coder"
)

func configSSH() *cobra.Command {
	var (
		sshConfigFile    string
		coderConfigFile  string
		sshOptions       []string
		showDiff         bool
		skipProxyCommand bool

		// Diff should exit with status 1 when files differ.
		filesDiffer bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "config-ssh",
		Short:       "Populate your SSH config with Host entries for all of your workspaces",
		Example: `
  - You can use -o (or --ssh-option) so set SSH options to be used for all your
    workspaces.

    ` + cliui.Styles.Code.Render("$ coder config-ssh -o ForwardAgent=yes") + `

  - You can use -D (or --diff) to display the changes that will be made.

    ` + cliui.Styles.Code.Render("$ coder config-ssh --diff"),
		PostRun: func(cmd *cobra.Command, args []string) {
			// TODO(mafredri): Should we refactor this.. e.g. sentinel error?
			if showDiff && filesDiffer {
				os.Exit(1) //nolint: revive
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}

			// Early check for workspaces to ensure API key has not expired.
			workspaces, err := client.Workspaces(cmd.Context(), codersdk.WorkspaceFilter{
				Owner: codersdk.Me,
			})
			if err != nil {
				return err
			}

			dirname, err := os.UserHomeDir()
			if err != nil {
				return xerrors.Errorf("user home dir failed: %w", err)
			}

			sshConfigFileOrig := sshConfigFile // Store the pre ~/ replacement name for serializing options.
			if strings.HasPrefix(sshConfigFile, "~/") {
				sshConfigFile = filepath.Join(dirname, sshConfigFile[2:])
			}
			coderConfigFileOrig := coderConfigFile
			coderConfigFile = filepath.Join(dirname, coderConfigFile[2:]) // Replace ~/ with home dir.

			// TODO(mafredri): Check coderConfigFile for previous options
			// coderConfigFile.

			// Only allow not-exist errors to avoid trashing
			// the users SSH config.
			configRaw, err := os.ReadFile(sshConfigFile)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("read ssh config failed: %w", err)
			}

			coderConfigRaw, err := os.ReadFile(coderConfigFile)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("read ssh config failed: %w", err)
			}

			// Keep track of changes we are making.
			var changes []string

			// Check for presence of old config format and
			// remove if present.
			configModified, ok := stripOldConfigBlock(configRaw)
			if ok {
				changes = append(changes, fmt.Sprintf("Remove old config block (START-CODER/END-CODER) from %s", sshConfigFileOrig))
			}

			if found := bytes.Index(configModified, []byte(sshConfigIncludeStatement)); found == -1 || (found > 0 && configModified[found-1] != '\n') {
				changes = append(changes, fmt.Sprintf("Add 'Include coder' to %s", sshConfigFileOrig))
				// Separate Include statement from user content with an empty newline.
				configModified = bytes.TrimRight(configModified, "\n")
				sep := "\n\n"
				if len(configModified) == 0 {
					sep = ""
				}
				configModified = append(configModified, []byte(sep+sshConfigIncludeStatement+"\n")...)
			}

			binaryFile, err := currentBinPath(cmd)
			if err != nil {
				return err
			}

			root := createConfig(cmd)
			var errGroup errgroup.Group
			type workspaceConfig struct {
				Name  string
				Hosts []string
			}
			workspaceConfigs := make([]workspaceConfig, len(workspaces))
			for i, workspace := range workspaces {
				i := i
				workspace := workspace
				errGroup.Go(func() error {
					resources, err := client.TemplateVersionResources(cmd.Context(), workspace.LatestBuild.TemplateVersionID)
					if err != nil {
						return err
					}

					wc := workspaceConfig{Name: workspace.Name}
					for _, resource := range resources {
						if resource.Transition != codersdk.WorkspaceTransitionStart {
							continue
						}
						for _, agent := range resource.Agents {
							hostname := workspace.Name
							if len(resource.Agents) > 1 {
								hostname += "." + agent.Name
							}
							wc.Hosts = append(wc.Hosts, hostname)
						}
					}
					workspaceConfigs[i] = wc

					return nil
				})
			}
			err = errGroup.Wait()
			if err != nil {
				return err
			}

			buf := &bytes.Buffer{}
			_, _ = buf.WriteString(sshCoderConfigHeader)

			// Store the provided flags as part of the
			// config for future (re)use.
			_, _ = buf.WriteString(sshCoderConfigOptionsHeader)
			if sshConfigFileOrig != sshDefaultConfigFileName {
				_, _ = fmt.Fprintf(buf, "# :%s=%s\n", "ssh-config-file", sshConfigFileOrig)
			}
			for _, opt := range sshOptions {
				_, _ = fmt.Fprintf(buf, "# :%s=%s\n", "ssh-option", opt)
			}
			_, _ = buf.WriteString("#\n")

			// Ensure stable sorting of output.
			slices.SortFunc(workspaceConfigs, func(a, b workspaceConfig) bool {
				return a.Name < b.Name
			})
			for _, wc := range workspaceConfigs {
				sort.Strings(wc.Hosts)
				// Write agent configuration.
				for _, hostname := range wc.Hosts {
					configOptions := []string{
						"Host coder." + hostname,
					}
					for _, option := range sshOptions {
						configOptions = append(configOptions, "\t"+option)
					}
					configOptions = append(configOptions,
						"\tHostName coder."+hostname,
						"\tConnectTimeout=0",
						"\tStrictHostKeyChecking=no",
						// Without this, the "REMOTE HOST IDENTITY CHANGED"
						// message will appear.
						"\tUserKnownHostsFile=/dev/null",
						// This disables the "Warning: Permanently added 'hostname' (RSA) to the list of known hosts."
						// message from appearing on every SSH. This happens because we ignore the known hosts.
						"\tLogLevel ERROR",
					)
					if !skipProxyCommand {
						configOptions = append(configOptions, fmt.Sprintf("\tProxyCommand %q --global-config %q ssh --stdio %s", binaryFile, root, hostname))
					}

					_, _ = buf.WriteString(strings.Join(configOptions, "\n"))
					_ = buf.WriteByte('\n')
				}
			}

			modifyCoderConfig := !bytes.Equal(coderConfigRaw, buf.Bytes())
			if modifyCoderConfig {
				if len(coderConfigRaw) == 0 {
					changes = append(changes, fmt.Sprintf("Write auto-generated coder config file to %s", coderConfigFileOrig))
				} else {
					changes = append(changes, fmt.Sprintf("Update auto-generated coder config file in %s", coderConfigFileOrig))
				}
			}

			if showDiff {
				if len(changes) > 0 {
					// Write to stderr to avoid dirtying the diff output.
					_, _ = fmt.Fprint(cmd.ErrOrStderr(), "Changes:\n\n")
					for _, change := range changes {
						_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "* %s\n", change)
					}
				}

				for _, diffFn := range []func() ([]byte, error){
					func() ([]byte, error) { return diffBytes(sshConfigFile, configRaw, configModified) },
					func() ([]byte, error) { return diffBytes(coderConfigFile, coderConfigRaw, buf.Bytes()) },
				} {
					diff, err := diffFn()
					if err != nil {
						return xerrors.Errorf("diff failed: %w", err)
					}
					if len(diff) > 0 {
						filesDiffer = true
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%s", diff)
					}
				}

				return nil
			}

			if len(changes) > 0 {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("The following changes will be made to your SSH configuration (use --diff to see changes):\n\n  * %s\n\n  Continue?", strings.Join(changes, "\n  * ")),
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprint(cmd.OutOrStdout(), "\n")

				if !bytes.Equal(configRaw, configModified) {
					err = writeWithTempFileAndMove(sshConfigFile, bytes.NewReader(configRaw))
					if err != nil {
						return xerrors.Errorf("write ssh config failed: %w", err)
					}
				}
				if modifyCoderConfig {
					err := writeWithTempFileAndMove(coderConfigFile, buf)
					if err != nil {
						return xerrors.Errorf("write coder ssh config failed: %w", err)
					}
				}
			}

			if len(workspaces) > 0 {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "You should now be able to ssh into your workspace")
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "For example, try running:\n\n\t$ ssh coder.%s\n\n", workspaces[0].Name)
			} else {
				_, _ = fmt.Fprint(cmd.OutOrStdout(), "You don't have any workspaces yet, try creating one with:\n\n\t$ coder create <workspace>\n\n")
			}
			return nil
		},
	}
	cliflag.StringVarP(cmd.Flags(), &sshConfigFile, "ssh-config-file", "", "CODER_SSH_CONFIG_FILE", sshDefaultConfigFileName, "Specifies the path to an SSH config.")
	cmd.Flags().StringVar(&coderConfigFile, "ssh-coder-config-file", sshDefaultCoderConfigFileName, "Specifies the path to an Coder SSH config file. Useful for testing.")
	_ = cmd.Flags().MarkHidden("ssh-coder-config-file")
	cmd.Flags().StringArrayVarP(&sshOptions, "ssh-option", "o", []string{}, "Specifies additional SSH options to embed in each host stanza.")
	cmd.Flags().BoolVarP(&showDiff, "diff", "D", false, "Show diff of changes that will be made.")
	cmd.Flags().BoolVarP(&skipProxyCommand, "skip-proxy-command", "", false, "Specifies whether the ProxyCommand option should be skipped. Useful for testing.")
	_ = cmd.Flags().MarkHidden("skip-proxy-command")

	return cmd
}

// writeWithTempFileAndMove writes to a temporary file in the same
// directory as path and renames the temp file to the file provided in
// path. This ensure we avoid trashing the file we are writing due to
// unforeseen circumstance like filesystem full, command killed, etc.
func writeWithTempFileAndMove(path string, r io.Reader) error {
	dir := filepath.Dir(path)
	name := filepath.Base(path)

	// Create a tempfile in the same directory for ensuring write
	// operation does not fail.
	f, err := os.CreateTemp(dir, fmt.Sprintf(".%s.", name))
	if err != nil {
		return xerrors.Errorf("create temp file failed: %w", err)
	}

	_, err = io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		return err
	}

	err = f.Close()
	if err != nil {
		return xerrors.Errorf("close temp file failed: %w", err)
	}

	err = os.Rename(f.Name(), path)
	if err != nil {
		return xerrors.Errorf("rename temp file failed: %w", err)
	}

	return nil
}

// currentBinPath returns the path to the coder binary suitable for use in ssh
// ProxyCommand.
func currentBinPath(cmd *cobra.Command) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", xerrors.Errorf("get executable path: %w", err)
	}

	binName := filepath.Base(exePath)
	// We use safeexec instead of os/exec because os/exec returns paths in
	// the current working directory, which we will run into very often when
	// looking for our own path.
	pathPath, err := safeexec.LookPath(binName)
	// On Windows, the coder-cli executable must be in $PATH for both Msys2/Git
	// Bash and OpenSSH for Windows (used by Powershell and VS Code) to function
	// correctly. Check if the current executable is in $PATH, and warn the user
	// if it isn't.
	if err != nil && runtime.GOOS == "windows" {
		cliui.Warn(cmd.OutOrStdout(),
			"The current executable is not in $PATH.",
			"This may lead to problems connecting to your workspace via SSH.",
			fmt.Sprintf("Please move %q to a location in your $PATH (such as System32) and run `%s config-ssh` again.", binName, binName),
		)
		// Return the exePath so SSH at least works outside of Msys2.
		return exePath, nil
	}

	// Warn the user if the current executable is not the same as the one in
	// $PATH.
	if filepath.Clean(pathPath) != filepath.Clean(exePath) {
		cliui.Warn(cmd.OutOrStdout(),
			"The current executable path does not match the executable path found in $PATH.",
			"This may cause issues connecting to your workspace via SSH.",
			fmt.Sprintf("\tCurrent executable path: %q", exePath),
			fmt.Sprintf("\tExecutable path in $PATH: %q", pathPath),
		)
	}

	return binName, nil
}

// diffBytes takes two byte slices and diffs them as if they were in a
// file named name.
func diffBytes(name string, b1, b2 []byte) ([]byte, error) {
	var buf bytes.Buffer
	var opts []write.Option
	// TODO(mafredri): Toggle color on/off
	if false {
		opts = append(opts, write.TerminalColor())
	}
	err := diff.Text(name, name+".new", b1, b2, &buf, opts...)
	if err != nil {
		return nil, err
	}
	b := buf.Bytes()
	// Check if diff only output two lines, if yes, there's no diff.
	//
	// Example:
	// 	--- /home/user/.ssh/config
	// 	+++ /home/user/.ssh/config.new
	if bytes.Count(b, []byte{'\n'}) == 2 {
		b = nil
	}
	return b, nil
}

// stripOldConfigBlock is here to migrate users from old config block
// format to new include statement.
func stripOldConfigBlock(data []byte) ([]byte, bool) {
	const (
		sshStartToken = "# ------------START-CODER-----------"
		sshEndToken   = "# ------------END-CODER------------"
	)

	startIndex := bytes.Index(data, []byte(sshStartToken))
	endIndex := bytes.Index(data, []byte(sshEndToken))
	if startIndex != -1 && endIndex != -1 {
		newdata := append([]byte{}, data[:startIndex-1]...)
		newdata = append(newdata, data[endIndex+len(sshEndToken):]...)
		return newdata, true
	}

	return data, false
}
