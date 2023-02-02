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
	sshStartToken            = "# ------------START-CODER-----------"
	sshEndToken              = "# ------------END-CODER------------"
	sshConfigSectionHeader   = "# This section is managed by coder. DO NOT EDIT."
	sshConfigDocsHeader      = `
#
# You should not hand-edit this section unless you are removing it, all
# changes will be lost when running "coder config-ssh".
`
	sshConfigOptionsHeader = `#
# Last config-ssh options:
`
)

// sshConfigOptions represents options that can be stored and read
// from the coder config in ~/.ssh/coder.
type sshConfigOptions struct {
	sshOptions []string
}

func (o sshConfigOptions) equal(other sshConfigOptions) bool {
	// Compare without side-effects or regard to order.
	opt1 := slices.Clone(o.sshOptions)
	sort.Strings(opt1)
	opt2 := slices.Clone(other.sshOptions)
	sort.Strings(opt2)
	return slices.Equal(opt1, opt2)
}

func (o sshConfigOptions) asList() (list []string) {
	for _, opt := range o.sshOptions {
		list = append(list, fmt.Sprintf("ssh-option: %s", opt))
	}
	return list
}

type sshWorkspaceConfig struct {
	Name  string
	Hosts []string
}

func sshFetchWorkspaceConfigs(ctx context.Context, client *codersdk.Client) ([]sshWorkspaceConfig, error) {
	res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
		Owner: codersdk.Me,
	})
	if err != nil {
		return nil, err
	}

	var errGroup errgroup.Group
	workspaceConfigs := make([]sshWorkspaceConfig, len(res.Workspaces))
	for i, workspace := range res.Workspaces {
		i := i
		workspace := workspace
		errGroup.Go(func() error {
			resources, err := client.TemplateVersionResources(ctx, workspace.LatestBuild.TemplateVersionID)
			if err != nil {
				return err
			}

			wc := sshWorkspaceConfig{Name: workspace.Name}
			var agents []codersdk.WorkspaceAgent
			for _, resource := range resources {
				if resource.Transition != codersdk.WorkspaceTransitionStart {
					continue
				}
				agents = append(agents, resource.Agents...)
			}

			// handle both WORKSPACE and WORKSPACE.AGENT syntax
			if len(agents) == 1 {
				wc.Hosts = append(wc.Hosts, workspace.Name)
			}
			for _, agent := range agents {
				hostname := workspace.Name + "." + agent.Name
				wc.Hosts = append(wc.Hosts, hostname)
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
}

func sshPrepareWorkspaceConfigs(ctx context.Context, client *codersdk.Client) (receive func() ([]sshWorkspaceConfig, error)) {
	wcC := make(chan []sshWorkspaceConfig, 1)
	errC := make(chan error, 1)
	go func() {
		wc, err := sshFetchWorkspaceConfigs(ctx, client)
		wcC <- wc
		errC <- err
	}()
	return func() ([]sshWorkspaceConfig, error) {
		return <-wcC, <-errC
	}
}

func configSSH() *cobra.Command {
	var (
		sshConfigFile    string
		sshConfigOpts    sshConfigOptions
		usePreviousOpts  bool
		dryRun           bool
		skipProxyCommand bool
	)
	cmd := &cobra.Command{
		Annotations: workspaceCommand,
		Use:         "config-ssh",
		Short:       "Add an SSH Host entry for your workspaces \"ssh coder.workspace\"",
		Example: formatExamples(
			example{
				Description: "You can use -o (or --ssh-option) so set SSH options to be used for all your workspaces",
				Command:     "coder config-ssh -o ForwardAgent=yes",
			},
			example{
				Description: "You can use --dry-run (or -n) to see the changes that would be made",
				Command:     "coder config-ssh --dry-run",
			},
		),
		Args: cobra.ExactArgs(0),
		RunE: func(cmd *cobra.Command, _ []string) error {
			client, err := CreateClient(cmd)
			if err != nil {
				return err
			}

			recvWorkspaceConfigs := sshPrepareWorkspaceConfigs(cmd.Context(), client)

			out := cmd.OutOrStdout()
			if dryRun {
				// Print everything except diff to stderr so
				// that it's possible to capture the diff.
				out = cmd.OutOrStderr()
			}
			coderBinary, err := currentBinPath(out)
			if err != nil {
				return err
			}
			escapedCoderBinary, err := sshConfigExecEscape(coderBinary)
			if err != nil {
				return xerrors.Errorf("escape coder binary for ssh failed: %w", err)
			}

			root := createConfig(cmd)
			escapedGlobalConfig, err := sshConfigExecEscape(string(root))
			if err != nil {
				return xerrors.Errorf("escape global config for ssh failed: %w", err)
			}

			homedir, err := os.UserHomeDir()
			if err != nil {
				return xerrors.Errorf("user home dir failed: %w", err)
			}

			if strings.HasPrefix(sshConfigFile, "~/") {
				sshConfigFile = filepath.Join(homedir, sshConfigFile[2:])
			}

			// Only allow not-exist errors to avoid trashing
			// the users SSH config.
			configRaw, err := os.ReadFile(sshConfigFile)
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return xerrors.Errorf("read ssh config failed: %w", err)
			}

			// Keep track of changes we are making.
			var changes []string

			// Parse the previous configuration only if config-ssh
			// has been run previously.
			var lastConfig *sshConfigOptions
			section, ok, err := sshConfigGetCoderSection(configRaw)
			if err != nil {
				return err
			}
			if ok {
				c := sshConfigParseLastOptions(bytes.NewReader(section))
				lastConfig = &c
			}

			// Avoid prompting in diff mode (unexpected behavior)
			// or when a previous config does not exist.
			if usePreviousOpts && lastConfig != nil {
				sshConfigOpts = *lastConfig
			} else if lastConfig != nil && !sshConfigOpts.equal(*lastConfig) {
				newOpts := sshConfigOpts.asList()
				newOptsMsg := "\n\n  New options: none"
				if len(newOpts) > 0 {
					newOptsMsg = fmt.Sprintf("\n\n  New options:\n    * %s", strings.Join(newOpts, "\n    * "))
				}
				oldOpts := lastConfig.asList()
				oldOptsMsg := "\n\n  Previous options: none"
				if len(oldOpts) > 0 {
					oldOptsMsg = fmt.Sprintf("\n\n  Previous options:\n    * %s", strings.Join(oldOpts, "\n    * "))
				}

				line, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("New options differ from previous options:%s%s\n\n  Use new options?", newOptsMsg, oldOptsMsg),
					IsConfirm: true,
				})
				if err != nil {
					if line == "" && xerrors.Is(err, cliui.Canceled) {
						return nil
					}
					// Selecting "no" will use the last config.
					sshConfigOpts = *lastConfig
				} else {
					changes = append(changes, "Use new SSH options")
				}
				// Only print when prompts are shown.
				if yes, _ := cmd.Flags().GetBool("yes"); !yes {
					_, _ = fmt.Fprint(out, "\n")
				}
			}

			configModified := configRaw

			buf := &bytes.Buffer{}
			before, _, after, err := sshConfigSplitOnCoderSection(configModified)
			if err != nil {
				return err
			}
			// Write the first half of the users config file to buf.
			_, _ = buf.Write(before)

			// Write comment and store the provided options as part
			// of the config for future (re)use.
			newline := len(before) > 0
			sshConfigWriteSectionHeader(buf, newline, sshConfigOpts)

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
					for _, option := range sshConfigOpts.sshOptions {
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
						configOptions = append(
							configOptions,
							fmt.Sprintf(
								"\tProxyCommand %s --global-config %s ssh --stdio %s",
								escapedCoderBinary, escapedGlobalConfig, hostname,
							),
						)
					}

					_, _ = buf.WriteString(strings.Join(configOptions, "\n"))
					_ = buf.WriteByte('\n')
				}
			}

			sshConfigWriteSectionEnd(buf)

			// Write the remainder of the users config file to buf.
			_, _ = buf.Write(after)

			if !bytes.Equal(configModified, buf.Bytes()) {
				changes = append(changes, fmt.Sprintf("Update the coder section in %s", sshConfigFile))
				configModified = buf.Bytes()
			}

			if len(changes) == 0 {
				_, _ = fmt.Fprintf(out, "No changes to make.\n")
				return nil
			}

			if dryRun {
				_, _ = fmt.Fprintf(out, "Dry run, the following changes would be made to your SSH configuration:\n\n  * %s\n\n", strings.Join(changes, "\n  * "))

				color := isTTYOut(cmd)
				diff, err := diffBytes(sshConfigFile, configRaw, configModified, color)
				if err != nil {
					return xerrors.Errorf("diff failed: %w", err)
				}
				if len(diff) > 0 {
					// Write diff to stdout.
					_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s", diff)
				}

				return nil
			}

			if len(changes) > 0 {
				_, err = cliui.Prompt(cmd, cliui.PromptOptions{
					Text:      fmt.Sprintf("The following changes will be made to your SSH configuration:\n\n    * %s\n\n  Continue?", strings.Join(changes, "\n    * ")),
					IsConfirm: true,
				})
				if err != nil {
					return nil
				}
				// Only print when prompts are shown.
				if yes, _ := cmd.Flags().GetBool("yes"); !yes {
					_, _ = fmt.Fprint(out, "\n")
				}
			}

			if !bytes.Equal(configRaw, configModified) {
				err = writeWithTempFileAndMove(sshConfigFile, bytes.NewReader(configModified))
				if err != nil {
					return xerrors.Errorf("write ssh config failed: %w", err)
				}
			}

			if len(workspaceConfigs) > 0 {
				_, _ = fmt.Fprintln(out, "You should now be able to ssh into your workspace.")
				_, _ = fmt.Fprintf(out, "For example, try running:\n\n\t$ ssh coder.%s\n", workspaceConfigs[0].Name)
			} else {
				_, _ = fmt.Fprint(out, "You don't have any workspaces yet, try creating one with:\n\n\t$ coder create <workspace>\n")
			}
			return nil
		},
	}
	cliflag.StringVarP(cmd.Flags(), &sshConfigFile, "ssh-config-file", "", "CODER_SSH_CONFIG_FILE", sshDefaultConfigFileName, "Specifies the path to an SSH config.")
	cmd.Flags().StringArrayVarP(&sshConfigOpts.sshOptions, "ssh-option", "o", []string{}, "Specifies additional SSH options to embed in each host stanza.")
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "Perform a trial run with no changes made, showing a diff at the end.")
	cmd.Flags().BoolVarP(&skipProxyCommand, "skip-proxy-command", "", false, "Specifies whether the ProxyCommand option should be skipped. Useful for testing.")
	_ = cmd.Flags().MarkHidden("skip-proxy-command")
	cliflag.BoolVarP(cmd.Flags(), &usePreviousOpts, "use-previous-options", "", "CODER_SSH_USE_PREVIOUS_OPTIONS", false, "Specifies whether or not to keep options from previous run of config-ssh.")
	cliui.AllowSkipPrompt(cmd)

	return cmd
}

//nolint:revive
func sshConfigWriteSectionHeader(w io.Writer, addNewline bool, o sshConfigOptions) {
	nl := "\n"
	if !addNewline {
		nl = ""
	}
	_, _ = fmt.Fprint(w, nl+sshStartToken+"\n")
	_, _ = fmt.Fprint(w, sshConfigSectionHeader)
	_, _ = fmt.Fprint(w, sshConfigDocsHeader)
	if len(o.sshOptions) > 0 {
		_, _ = fmt.Fprint(w, sshConfigOptionsHeader)
		for _, opt := range o.sshOptions {
			_, _ = fmt.Fprintf(w, "# :%s=%s\n", "ssh-option", opt)
		}
	}
	_, _ = fmt.Fprint(w, "#\n")
}

func sshConfigWriteSectionEnd(w io.Writer) {
	_, _ = fmt.Fprint(w, sshEndToken+"\n")
}

func sshConfigParseLastOptions(r io.Reader) (o sshConfigOptions) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "# :") {
			line = strings.TrimPrefix(line, "# :")
			parts := strings.SplitN(line, "=", 2)
			switch parts[0] {
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

// sshConfigGetCoderSection is a helper function that only returns the coder
// section of the SSH config and a boolean if it exists.
func sshConfigGetCoderSection(data []byte) (section []byte, ok bool, err error) {
	_, section, _, err = sshConfigSplitOnCoderSection(data)
	if err != nil {
		return nil, false, err
	}

	return section, len(section) > 0, nil
}

// sshConfigSplitOnCoderSection splits the SSH config into 3 sections.
// All lines before sshStartToken, the coder section, and all lines after
// sshEndToken.
func sshConfigSplitOnCoderSection(data []byte) (before, section []byte, after []byte, err error) {
	startCount := bytes.Count(data, []byte(sshStartToken))
	endCount := bytes.Count(data, []byte(sshEndToken))
	if startCount > 1 || endCount > 1 {
		return nil, nil, nil, xerrors.New("Malformed config: ssh config has multiple coder sections, please remove all but one")
	}

	startIndex := bytes.Index(data, []byte(sshStartToken))
	endIndex := bytes.Index(data, []byte(sshEndToken))
	if startIndex == -1 && endIndex != -1 {
		return nil, nil, nil, xerrors.New("Malformed config: ssh config has end header, but missing start header")
	}
	if startIndex != -1 && endIndex == -1 {
		return nil, nil, nil, xerrors.New("Malformed config: ssh config has start header, but missing end header")
	}
	if startIndex != -1 && endIndex != -1 {
		if startIndex > endIndex {
			return nil, nil, nil, xerrors.New("Malformed config: ssh config has coder section, but it is malformed and the END header is before the START header")
		}
		// We use -1 and +1 here to also include the preceding
		// and trailing newline, where applicable.
		start := startIndex
		if start > 0 {
			start--
		}
		end := endIndex + len(sshEndToken)
		if end < len(data) {
			end++
		}
		return data[:start], data[start:end], data[end:], nil
	}

	return data, nil, nil, nil
}

// writeWithTempFileAndMove writes to a temporary file in the same
// directory as path and renames the temp file to the file provided in
// path. This ensure we avoid trashing the file we are writing due to
// unforeseen circumstance like filesystem full, command killed, etc.
func writeWithTempFileAndMove(path string, r io.Reader) (err error) {
	dir := filepath.Dir(path)
	name := filepath.Base(path)

	// Ensure that e.g. the ~/.ssh directory exists.
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return xerrors.Errorf("create directory: %w", err)
	}

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

// sshConfigExecEscape quotes the string if it contains spaces, as per
// `man 5 ssh_config`. However, OpenSSH uses exec in the users shell to
// run the command, and as such the formatting/escape requirements
// cannot simply be covered by `fmt.Sprintf("%q", path)`.
//
// Always escaping the path with `fmt.Sprintf("%q", path)` usually works
// on most platforms, but double quotes sometimes break on Windows 10
// (see #2853). This function takes a best-effort approach to improving
// compatibility and covering edge cases.
//
// Given the following ProxyCommand:
//
//	ProxyCommand "/path/with space/coder" ssh --stdio work
//
// This is ~what OpenSSH would execute:
//
//	/bin/bash -c '"/path/with space/to/coder" ssh --stdio workspace'
//
// However, since it's actually an arg in C, the contents inside the
// single quotes are interpreted as is, e.g. if there was a '\t', it
// would be the literal string '\t', not a tab.
//
// See:
//   - https://github.com/coder/coder/issues/2853
//   - https://github.com/openssh/openssh-portable/blob/V_9_0_P1/sshconnect.c#L158-L167
//   - https://github.com/PowerShell/openssh-portable/blob/v8.1.0.0/sshconnect.c#L231-L293
//   - https://github.com/PowerShell/openssh-portable/blob/v8.1.0.0/contrib/win32/win32compat/w32fd.c#L1075-L1100
func sshConfigExecEscape(path string) (string, error) {
	// This is unlikely to ever happen, but newlines are allowed on
	// certain filesystems, but cannot be used inside ssh config.
	if strings.ContainsAny(path, "\n") {
		return "", xerrors.Errorf("invalid path: %s", path)
	}
	// In the unlikely even that a path contains quotes, they must be
	// escaped so that they are not interpreted as shell quotes.
	if strings.Contains(path, "\"") {
		path = strings.ReplaceAll(path, "\"", "\\\"")
	}
	// A space or a tab requires quoting, but tabs must not be escaped
	// (\t) since OpenSSH interprets it as a literal \t, not a tab.
	if strings.ContainsAny(path, " \t") {
		path = fmt.Sprintf("\"%s\"", path) //nolint:gocritic // We don't want %q here.
	}
	return path, nil
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

	return exePath, nil
}

// diffBytes takes two byte slices and diffs them as if they were in a
// file named name.
// nolint: revive // Color is an option, not a control coupling.
func diffBytes(name string, b1, b2 []byte, color bool) ([]byte, error) {
	var buf bytes.Buffer
	var opts []write.Option
	if color {
		opts = append(opts, write.TerminalColor())
	}
	err := diff.Text(name, name, b1, b2, &buf, opts...)
	if err != nil {
		return nil, err
	}
	b := buf.Bytes()
	// Check if diff only output two lines, if yes, there's no diff.
	//
	// Example:
	// 	--- /home/user/.ssh/config
	// 	+++ /home/user/.ssh/config
	if bytes.Count(b, []byte{'\n'}) == 2 {
		b = nil
	}
	return b, nil
}
