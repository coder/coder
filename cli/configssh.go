package cli

import (
	"bufio"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/cli/safeexec"
	"github.com/spf13/cobra"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

const (
	// Include path is relative to `~/.ssh` and each workspace will
	// have a separate file (e.g. `~/.ssh/coder.d/host-my-workspace`).
	// By prefixing hosts as `host-` we give ourselves the flexibility
	// to manage other files in this folder as well, e.g. keys, vscode
	// specific config (i.e. for only listing coder files in vscode),
	// etc.
	sshEnabledLine = "Include coder.d/host-*"
	// TODO(mafredri): Does this hold on Windows?
	sshCoderConfigd = "~/.ssh/coder.d"
	// TODO(mafredri): Write a README to the folder?
	// sshCoderConfigdReadme = `Information, tricks, removal, etc.`
)

func configSSH() *cobra.Command {
	var (
		sshConfigFile    string
		sshOptions       []string
		skipProxyCommand bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "config-ssh",
		Short:       "Populate your SSH config with Host entries for all of your workspaces",
		Example: `
  - You can use -o (or --ssh-option) so set SSH options to be used for all your
    workspaces.

    ` + cliui.Styles.Code.Render("$ coder config-ssh -o ForwardAgent=yes"),
		RunE: func(cmd *cobra.Command, args []string) error {
			client, err := createClient(cmd)
			if err != nil {
				return err
			}
			organization, err := currentOrganization(cmd, client)
			if err != nil {
				return err
			}
			dirname, _ := os.UserHomeDir()
			if strings.HasPrefix(sshConfigFile, "~/") {
				sshConfigFile = filepath.Join(dirname, sshConfigFile[2:])
			}
			confd := filepath.Join(dirname, sshCoderConfigd[2:])

			workspaces, err := client.WorkspacesByOwner(cmd.Context(), organization.ID, codersdk.Me)
			if err != nil {
				return err
			}
			if len(workspaces) == 0 {
				return xerrors.New("You don't have any workspaces!")
			}

			binaryFile, err := currentBinPath(cmd)
			if err != nil {
				return err
			}

			enabled := false
			err = func() error {
				exists := true
				configRaw, err := os.Open(sshConfigFile)
				if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
					return err
				} else if xerrors.Is(err, fs.ErrNotExist) {
					exists = false
				}
				defer configRaw.Close()

				if exists {
					s := bufio.NewScanner(configRaw)
					for s.Scan() {
						if strings.HasPrefix(s.Text(), sshEnabledLine) {
							enabled = true
							break
						}
					}
					if s.Err() != nil {
						return err
					}
				}
				return nil
			}()
			if err != nil {
				return err
			}

			if !enabled {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("The following line will be added to %s:\n\n    %s\n\n  And configuration files will be stored in ~/.ssh/coder.d\n\n  Continue?", sshConfigFile, sshEnabledLine),
					IsConfirm: true,
				})
				if err != nil {
					return err
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n")

				// Create directory first in case of error since we do not check for existence.
				err = os.Mkdir(confd, 0o700)
				if err != nil && !xerrors.Is(err, fs.ErrExist) {
					return err
				}

				f, err := os.OpenFile(sshConfigFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
				if err != nil {
					return err
				}

				// TODO(mafredri): Only add newline if necessary.
				_, err = f.WriteString("\n" + sshEnabledLine)
				if err != nil {
					return err
				}

				err = f.Close()
				if err != nil {
					return err
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "* Added Include directive to %s\n", sshConfigFile)
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "* Created configuration directory %s\n", confd)
			}

			root := createConfig(cmd)
			var errGroup errgroup.Group
			// TODO(mafredri): Delete configurations that no longer exist.
			for _, workspace := range workspaces {
				workspace := workspace
				errGroup.Go(func() error {
					resources, err := client.TemplateVersionResources(cmd.Context(), workspace.LatestBuild.TemplateVersionID)
					if err != nil {
						return err
					}
					for _, resource := range resources {
						if resource.Transition != codersdk.WorkspaceTransitionStart {
							continue
						}
						for _, agent := range resource.Agents {
							hostname := workspace.Name
							if len(resource.Agents) > 1 {
								hostname += "." + agent.Name
							}
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

							dest := filepath.Join(confd, fmt.Sprintf("host-%s", hostname))
							// TODO(mafredri): Avoid re-write if files match.
							err := os.WriteFile(dest, []byte(strings.Join(configOptions, "\n")), 0o600)
							if err != nil {
								return err
							}
						}
					}
					return nil
				})
			}
			err = errGroup.Wait()
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "* Created workspace configurations in %s\n\n", confd)
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "You should now be able to ssh into your workspace.")
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "For example, try running:\n\n\t$ ssh coder.%s\n\n", workspaces[0].Name)
			return nil
		},
	}
	cliflag.StringVarP(cmd.Flags(), &sshConfigFile, "ssh-config-file", "", "CODER_SSH_CONFIG_FILE", "~/.ssh/config", "Specifies the path to an SSH config.")
	cmd.Flags().StringArrayVarP(&sshOptions, "ssh-option", "o", []string{}, "Specifies additional SSH options to embed in each host stanza.")
	cmd.Flags().BoolVarP(&skipProxyCommand, "skip-proxy-command", "", false, "Specifies whether the ProxyCommand option should be skipped. Useful for testing.")
	_ = cmd.Flags().MarkHidden("skip-proxy-command")

	return cmd
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
