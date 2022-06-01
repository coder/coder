package cli

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
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
	sshDefaultConfigFileName      = "~/.ssh/config"
	sshDefaultCoderConfigFileName = "~/.ssh/coder"
	sshCoderConfigHeader          = "# This file is managed by coder. DO NOT EDIT."
	sshCoderConfigDocsHeader      = `
#
# You should not hand-edit this file, all changes will be lost upon workspace
# creation, deletion or when running "coder config-ssh".`
	sshCoderConfigOptionsHeader = `
#
# Last config-ssh options:
`
	// Relative paths are assumed to be in ~/.ssh, except when
	// included in /etc/ssh.
	sshConfigIncludeStatement = "Include coder"
)

// Regular expressions are used because SSH configs do not have
// meaningful indentation and keywords are case-insensitive.
var (
	// Find the first Host and Match statement as these restrict the
	// following declarations to be used conditionally.
	sshHostRe = regexp.MustCompile(`(?m)^[\t ]*((?i)Host|Match)\s[^\n\r]*$`)
	// Find the semantically correct include statement. Since the user can
	// modify their configuration as they see fit, there could be:
	// - Leading indentation (space, tab)
	// - Trailing indentation (space, tab), followed by e.g. a comment or
	//   another file to Include (we don't want to support this, but
	//   explicitly blocking it adds complexity)
	// - Select newline after Include statement for removal purposes
	sshCoderIncludedRe = regexp.MustCompile(`(?m)^[\t ]*((?i)Include) coder([\t ].*)?[\r]?[\n]?$`)
)

// sshCoderConfigOptions represents options that can be stored and read
// from the coder config in ~/.ssh/coder.
type sshCoderConfigOptions struct {
	sshConfigDefaultFile string
	sshConfigFile        string
	sshOptions           []string
}

func (o sshCoderConfigOptions) equal(other sshCoderConfigOptions) bool {
	// Compare without side-effects or regard to order.
	opt1 := slices.Clone(o.sshOptions)
	sort.Strings(opt1)
	opt2 := slices.Clone(other.sshOptions)
	sort.Strings(opt2)
	return o.sshConfigFile == other.sshConfigFile && slices.Equal(opt1, opt2)
}

func (o sshCoderConfigOptions) asArgs() (args []string) {
	if o.sshConfigFile != o.sshConfigDefaultFile {
		args = append(args, "--ssh-config-file", o.sshConfigFile)
	}
	for _, opt := range o.sshOptions {
		args = append(args, "--ssh-option", fmt.Sprintf("%q", opt))
	}
	return args
}

func (o sshCoderConfigOptions) asList() (list []string) {
	if o.sshConfigFile != o.sshConfigDefaultFile {
		list = append(list, fmt.Sprintf("ssh-config-file: %s", o.sshConfigFile))
	}
	for _, opt := range o.sshOptions {
		list = append(list, fmt.Sprintf("ssh-option: %s", opt))
	}
	return list
}

type sshWorkspaceConfig struct {
	Name  string
	Hosts []string
}

func sshPrepareWorkspaceConfigs(ctx context.Context, client *codersdk.Client) (receive func() ([]sshWorkspaceConfig, error)) {
	wcC := make(chan []sshWorkspaceConfig, 1)
	errC := make(chan error, 1)
	go func() {
		wc, err := func() ([]sshWorkspaceConfig, error) {
			workspaces, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				Owner: codersdk.Me,
			})
			if err != nil {
				return nil, err
			}

			var errGroup errgroup.Group
			workspaceConfigs := make([]sshWorkspaceConfig, len(workspaces))
			for i, workspace := range workspaces {
				i := i
				workspace := workspace
				errGroup.Go(func() error {
					resources, err := client.TemplateVersionResources(ctx, workspace.LatestBuild.TemplateVersionID)
					if err != nil {
						return err
					}

					wc := sshWorkspaceConfig{Name: workspace.Name}
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
				return nil, err
			}

			return workspaceConfigs, nil
		}()
		wcC <- wc
		errC <- err
	}()
	return func() ([]sshWorkspaceConfig, error) {
		return <-wcC, <-errC
	}
}

func configSSH() *cobra.Command {
	var (
		coderConfig      sshCoderConfigOptions
		coderConfigFile  string
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

			recvWorkspaceConfigs := sshPrepareWorkspaceConfigs(cmd.Context(), client)

			out := cmd.OutOrStdout()
			if showDiff {
				out = cmd.OutOrStderr()
			}
			binaryFile, err := currentBinPath(out)
			if err != nil {
				return err
			}

			dirname, err := os.UserHomeDir()
			if err != nil {
				return xerrors.Errorf("user home dir failed: %w", err)
			}

			sshConfigFile := coderConfig.sshConfigFile
			if strings.HasPrefix(sshConfigFile, "~/") {
				sshConfigFile = filepath.Join(dirname, sshConfigFile[2:])
			}
			if strings.HasPrefix(coderConfigFile, "~/") {
				coderConfigFile = filepath.Join(dirname, coderConfigFile[2:])
			}

			// Only allow not-exist errors to avoid trashing
			// the users SSH config.
			configRaw, err := os.ReadFile(sshConfigFile)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("read ssh config failed: %w", err)
			}

			coderConfigExists := true
			coderConfigRaw, err := os.ReadFile(coderConfigFile)
			if err != nil {
				//nolint: revive // Inverting this if statement doesn't improve readability.
				if errors.Is(err, fs.ErrNotExist) {
					coderConfigExists = false
				} else {
					return xerrors.Errorf("read ssh config failed: %w", err)
				}
			}
			if len(coderConfigRaw) > 0 {
				if !bytes.HasPrefix(coderConfigRaw, []byte(sshCoderConfigHeader)) {
					return xerrors.Errorf("unexpected content in %s: remove the file and rerun the command to continue", coderConfigFile)
				}
			}
			lastCoderConfig := sshCoderConfigParseLastOptions(bytes.NewReader(coderConfigRaw), coderConfig.sshConfigDefaultFile)

			// Avoid prompting in diff mode (unexpected behavior)
			// or when a previous config does not exist.
			if !showDiff && !coderConfig.equal(lastCoderConfig) && coderConfigExists {
				newOpts := coderConfig.asList()
				newOptsMsg := "\n\n  New options: none"
				if len(newOpts) > 0 {
					newOptsMsg = fmt.Sprintf("\n\n  New options:\n    * %s", strings.Join(newOpts, "\n    * "))
				}
				oldOpts := lastCoderConfig.asList()
				oldOptsMsg := "\n\n  Previous options: none"
				if len(oldOpts) > 0 {
					oldOptsMsg = fmt.Sprintf("\n\n  Previous options:\n    * %s", strings.Join(oldOpts, "\n    * "))
				}

				line, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("New options differ from previous options:%s%s\n\n  Use new options?", newOptsMsg, oldOptsMsg),
					IsConfirm: true,
				})
				if err != nil {
					// TODO(mafredri): Better way to differ between "no" and Ctrl+C?
					if line == "" && xerrors.Is(err, cliui.Canceled) {
						return nil
					}
					// Selecting "no" will use the last config.
					coderConfig = lastCoderConfig
				}
				_, _ = fmt.Fprint(out, "\n")
			}

			// Keep track of changes we are making.
			var changes []string

			// Check for presence of old config format and
			// remove if present.
			configModified, ok := stripOldConfigBlock(configRaw)
			if ok {
				changes = append(changes, fmt.Sprintf("Remove old config block (START-CODER/END-CODER) from %s", sshConfigFile))
			}

			// Check for the presence of the coder Include
			// statement is present and add if missing.
			configModified, ok = sshConfigAddCoderInclude(configModified)
			if ok {
				changes = append(changes, fmt.Sprintf("Add %q to %s", "Include coder", sshConfigFile))
			}

			root := createConfig(cmd)

			buf := &bytes.Buffer{}

			// Write header and store the provided options as part
			// of the config for future (re)use.
			err = sshCoderConfigWriteHeader(buf, coderConfig)
			if err != nil {
				return xerrors.Errorf("write coder config header failed: %w", err)
			}

			workspaceConfigs, err := recvWorkspaceConfigs()
			if err != nil {
				return xerrors.Errorf("fetch workspace configs failed: %w", err)
			}
			// Ensure stable sorting of output.
			slices.SortFunc(workspaceConfigs, func(a, b sshWorkspaceConfig) bool {
				return a.Name < b.Name
			})
			for _, wc := range workspaceConfigs {
				sort.Strings(wc.Hosts)
				// Write agent configuration.
				for _, hostname := range wc.Hosts {
					configOptions := []string{
						"Host coder." + hostname,
					}
					for _, option := range coderConfig.sshOptions {
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
					changes = append(changes, fmt.Sprintf("Write auto-generated coder config file to %s", coderConfigFile))
				} else {
					changes = append(changes, fmt.Sprintf("Update auto-generated coder config file in %s", coderConfigFile))
				}
			}

			if showDiff {
				if len(changes) > 0 {
					// Write to stderr to avoid dirtying the diff output.
					_, _ = fmt.Fprint(out, "Changes:\n\n")
					for _, change := range changes {
						_, _ = fmt.Fprintf(out, "  * %s\n", change)
					}
				}

				color := isTTYOut(cmd)
				for _, diffFn := range []func() ([]byte, error){
					func() ([]byte, error) { return diffBytes(sshConfigFile, configRaw, configModified, color) },
					func() ([]byte, error) { return diffBytes(coderConfigFile, coderConfigRaw, buf.Bytes(), color) },
				} {
					diff, err := diffFn()
					if err != nil {
						return xerrors.Errorf("diff failed: %w", err)
					}
					if len(diff) > 0 {
						filesDiffer = true
						// Always write to stdout.
						_, _ = fmt.Fprintf(cmd.OutOrStdout(), "\n%s", diff)
					}
				}

				return nil
			}

			if len(changes) > 0 {
				// In diff mode we don't prompt re-using the previous
				// configuration, so we output the entire command.
				diffCommand := fmt.Sprintf("$ %s %s", cmd.CommandPath(), strings.Join(append(coderConfig.asArgs(), "--diff"), " "))
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("The following changes will be made to your SSH configuration:\n\n    * %s\n\n  To see changes, run diff:\n\n    %s\n\n  Continue?", strings.Join(changes, "\n    * "), diffCommand),
					IsConfirm: true,
				})
				if err != nil {
					return nil
				}
				_, _ = fmt.Fprint(out, "\n")

				if !bytes.Equal(configRaw, configModified) {
					err = writeWithTempFileAndMove(sshConfigFile, bytes.NewReader(configModified))
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

			if len(workspaceConfigs) > 0 {
				_, _ = fmt.Fprintln(out, "You should now be able to ssh into your workspace.")
				_, _ = fmt.Fprintf(out, "For example, try running:\n\n\t$ ssh coder.%s\n\n", workspaceConfigs[0].Name)
			} else {
				_, _ = fmt.Fprint(out, "You don't have any workspaces yet, try creating one with:\n\n\t$ coder create <workspace>\n\n")
			}
			return nil
		},
	}
	cliflag.StringVarP(cmd.Flags(), &coderConfig.sshConfigFile, "ssh-config-file", "", "CODER_SSH_CONFIG_FILE", sshDefaultConfigFileName, "Specifies the path to an SSH config.")
	cmd.Flags().StringVar(&coderConfig.sshConfigDefaultFile, "test.default-ssh-config-file", sshDefaultConfigFileName, "Specifies the default path to the SSH config file. Useful for testing.")
	_ = cmd.Flags().MarkHidden("test.default-ssh-config-file")
	cmd.Flags().StringVar(&coderConfigFile, "test.ssh-coder-config-file", sshDefaultCoderConfigFileName, "Specifies the path to an Coder SSH config file. Useful for testing.")
	_ = cmd.Flags().MarkHidden("test.ssh-coder-config-file")
	cmd.Flags().StringArrayVarP(&coderConfig.sshOptions, "ssh-option", "o", []string{}, "Specifies additional SSH options to embed in each host stanza.")
	cmd.Flags().BoolVarP(&showDiff, "diff", "D", false, "Show diff of changes that will be made.")
	cmd.Flags().BoolVarP(&skipProxyCommand, "skip-proxy-command", "", false, "Specifies whether the ProxyCommand option should be skipped. Useful for testing.")
	_ = cmd.Flags().MarkHidden("skip-proxy-command")

	return cmd
}

// sshConfigAddCoderInclude checks for the coder Include statement and
// returns modified = true if it was added.
func sshConfigAddCoderInclude(data []byte) (modifiedData []byte, modified bool) {
	valid := false
	firstHost := sshHostRe.FindIndex(data)
	coderInclude := sshCoderIncludedRe.FindIndex(data)
	if firstHost != nil && coderInclude != nil {
		// If the Coder Include statement exists
		// before a Host entry, we're good.
		valid = coderInclude[1] < firstHost[0]
		if !valid {
			// Remove invalid Include statement.
			d := append([]byte{}, data[:coderInclude[0]]...)
			d = append(d, data[coderInclude[1]:]...)
			data = d
		}
	} else if coderInclude != nil {
		valid = true
	}
	if valid {
		return data, false
	}

	// Add Include statement to the top of SSH config.
	// The user is allowed to move it as long as it
	// stays above the first Host (or Match) statement.
	sep := "\n\n"
	if len(data) == 0 {
		// If SSH config is empty, a single newline will suffice.
		sep = "\n"
	}
	data = append([]byte(sshConfigIncludeStatement+sep), data...)

	return data, true
}

func sshCoderConfigWriteHeader(w io.Writer, o sshCoderConfigOptions) error {
	_, _ = fmt.Fprint(w, sshCoderConfigHeader)
	_, _ = fmt.Fprint(w, sshCoderConfigDocsHeader)
	_, _ = fmt.Fprint(w, sshCoderConfigOptionsHeader)
	if o.sshConfigFile != o.sshConfigDefaultFile {
		_, _ = fmt.Fprintf(w, "# :%s=%s\n", "ssh-config-file", o.sshConfigFile)
	}
	for _, opt := range o.sshOptions {
		_, _ = fmt.Fprintf(w, "# :%s=%s\n", "ssh-option", opt)
	}
	_, _ = fmt.Fprint(w, "#\n")
	return nil
}

func sshCoderConfigParseLastOptions(r io.Reader, sshConfigDefaultFile string) (o sshCoderConfigOptions) {
	o.sshConfigDefaultFile = sshConfigDefaultFile
	o.sshConfigFile = sshConfigDefaultFile // Default value is not written.

	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "# :") {
			line = strings.TrimPrefix(line, "# :")
			parts := strings.SplitN(line, "=", 2)
			switch parts[0] {
			case "ssh-config-file":
				o.sshConfigFile = parts[1]
			case "ssh-option":
				o.sshOptions = append(o.sshOptions, parts[1])
			default:
				// Unknown option, ignore.
			}
		}
	}
	if err := s.Err(); err != nil {
		panic(err)
	}

	return o
}

// writeWithTempFileAndMove writes to a temporary file in the same
// directory as path and renames the temp file to the file provided in
// path. This ensure we avoid trashing the file we are writing due to
// unforeseen circumstance like filesystem full, command killed, etc.
func writeWithTempFileAndMove(path string, r io.Reader) (err error) {
	dir := filepath.Dir(path)
	name := filepath.Base(path)

	// Create a tempfile in the same directory for ensuring write
	// operation does not fail.
	f, err := os.CreateTemp(dir, fmt.Sprintf(".%s.", name))
	if err != nil {
		return xerrors.Errorf("create temp file failed: %w", err)
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name()) // Cleanup in case a step failed.
		}
	}()

	_, err = io.Copy(f, r)
	if err != nil {
		_ = f.Close()
		return xerrors.Errorf("write temp file failed: %w", err)
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
func currentBinPath(w io.Writer) (string, error) {
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
		cliui.Warn(w,
			"The current executable is not in $PATH.",
			"This may lead to problems connecting to your workspace via SSH.",
			fmt.Sprintf("Please move %q to a location in your $PATH (such as System32) and run `%s config-ssh` again.", binName, binName),
		)
		_, _ = fmt.Fprint(w, "\n")
		// Return the exePath so SSH at least works outside of Msys2.
		return exePath, nil
	}

	// Warn the user if the current executable is not the same as the one in
	// $PATH.
	if filepath.Clean(pathPath) != filepath.Clean(exePath) {
		cliui.Warn(w,
			"The current executable path does not match the executable path found in $PATH.",
			"This may cause issues connecting to your workspace via SSH.",
			fmt.Sprintf("\tCurrent executable path: %q", exePath),
			fmt.Sprintf("\tExecutable path in $PATH: %q", pathPath),
		)
		_, _ = fmt.Fprint(w, "\n")
	}

	return binName, nil
}

// diffBytes takes two byte slices and diffs them as if they were in a
// file named name.
//nolint: revive // Color is an option, not a control coupling.
func diffBytes(name string, b1, b2 []byte, color bool) ([]byte, error) {
	var buf bytes.Buffer
	var opts []write.Option
	if color {
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
