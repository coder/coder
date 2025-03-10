// Package cli provides the command line interface for Coder.
package cli

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/cli/safeexec"
	"github.com/natefinch/atomic"
	"github.com/pkg/diff"
	"github.com/pkg/diff/write"
	"golang.org/x/exp/constraints"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
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
	waitEnum           string
	userHostPrefix     string
	userHostnameSuffix string
	sshOptions         []string
	disableAutostart   bool
	header             []string
	headerCommand      string
	removedKeys        map[string]bool
}

// addOptions expects options in the form of "option=value" or "option value".
// It will override any existing option with the same key to prevent duplicates.
// Invalid options will return an error.
func (o *sshConfigOptions) addOptions(options ...string) error {
	for _, option := range options {
		err := o.addOption(option)
		if err != nil {
			return err
		}
	}
	return nil
}

func (o *sshConfigOptions) addOption(option string) error {
	key, value, err := codersdk.ParseSSHConfigOption(option)
	if err != nil {
		return err
	}
	lowerKey := strings.ToLower(key)
	if o.removedKeys != nil && o.removedKeys[lowerKey] {
		// Key marked as removed, skip.
		return nil
	}
	// Only append the option if it is not empty
	// (we interpret empty as removal).
	if value != "" {
		o.sshOptions = append(o.sshOptions, option)
	} else {
		if o.removedKeys == nil {
			o.removedKeys = make(map[string]bool)
		}
		o.removedKeys[lowerKey] = true
	}
	return nil
}

func (o sshConfigOptions) equal(other sshConfigOptions) bool {
	if !slicesSortedEqual(o.sshOptions, other.sshOptions) {
		return false
	}
	if !slicesSortedEqual(o.header, other.header) {
		return false
	}
	return o.waitEnum == other.waitEnum && 
		o.userHostPrefix == other.userHostPrefix && 
		o.userHostnameSuffix == other.userHostnameSuffix && 
		o.disableAutostart == other.disableAutostart && 
		o.headerCommand == other.headerCommand
}

// slicesSortedEqual compares two slices without side-effects or regard to order.
func slicesSortedEqual[S ~[]E, E constraints.Ordered](a, b S) bool {
	if len(a) != len(b) {
		return false
	}
	a = slices.Clone(a)
	slices.Sort(a)
	b = slices.Clone(b)
	slices.Sort(b)
	return slices.Equal(a, b)
}

func (o sshConfigOptions) asList() (list []string) {
	if o.waitEnum != "auto" {
		list = append(list, fmt.Sprintf("wait: %s", o.waitEnum))
	}
	if o.userHostPrefix != "" {
		list = append(list, fmt.Sprintf("ssh-host-prefix: %s", o.userHostPrefix))
	}
	if o.userHostnameSuffix != "" {
		list = append(list, fmt.Sprintf("ssh-hostname-suffix: %s", o.userHostnameSuffix))
	}
	if o.disableAutostart {
		list = append(list, fmt.Sprintf("disable-autostart: %v", o.disableAutostart))
	}
	for _, opt := range o.sshOptions {
		list = append(list, fmt.Sprintf("ssh-option: %s", opt))
	}
	for _, h := range o.header {
		list = append(list, fmt.Sprintf("header: %s", h))
	}
	if o.headerCommand != "" {
		list = append(list, fmt.Sprintf("header-command: %s", o.headerCommand))
	}

	return list
}

func (r *RootCmd) configSSH() *serpent.Command {
	var (
		sshConfigFile       string
		sshConfigOpts       sshConfigOptions
		usePreviousOpts     bool
		dryRun              bool
		skipProxyCommand    bool
		forceUnixSeparators bool
		coderCliPath        string
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Annotations: workspaceCommand,
		Use:         "config-ssh",
		Short:       "Add an SSH Host entry for your workspaces \"ssh coder.workspace\"",
		Long: FormatExamples(
			Example{
				Description: "You can use -o (or --ssh-option) so set SSH options to be used for all your workspaces",
				Command:     "coder config-ssh -o ForwardAgent=yes",
			},
			Example{
				Description: "You can use --dry-run (or -n) to see the changes that would be made",
				Command:     "coder config-ssh --dry-run",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			if sshConfigOpts.waitEnum != "auto" && skipProxyCommand {
				// The wait option is applied to the ProxyCommand. If the user
				// specifies skip-proxy-command, then wait cannot be applied.
				return xerrors.Errorf("cannot specify both --skip-proxy-command and --wait")
			}
			sshConfigOpts.header = r.header
			sshConfigOpts.headerCommand = r.headerCommand

			// Talk to the API early to prevent the version mismatch
			// warning from being printed in the middle of a prompt.
			// This is needed because the asynchronous requests issued
			// by sshPrepareWorkspaceConfigs may otherwise trigger the
			// warning at any time.
			_, _ = client.BuildInfo(ctx)

			out := inv.Stdout
			if dryRun {
				// Print everything except diff to stderr so
				// that it's possible to capture the diff.
				out = inv.Stderr
			}

			var err error
			coderBinary := coderCliPath
			if coderBinary == "" {
				coderBinary, err = currentBinPath(out)
				if err != nil {
					return err
				}
			}

			escapedCoderBinary, err := sshConfigExecEscape(coderBinary, forceUnixSeparators)
			if err != nil {
				return xerrors.Errorf("escape coder binary for ssh failed: %w", err)
			}

			root := r.createConfig()
			escapedGlobalConfig, err := sshConfigExecEscape(string(root), forceUnixSeparators)
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
				for _, v := range sshConfigOpts.sshOptions {
					// If the user passes an invalid option, we should catch
					// this early.
					if _, _, err := codersdk.ParseSSHConfigOption(v); err != nil {
						return xerrors.Errorf("invalid option from flag: %w", err)
					}
				}
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

				line, err := cliui.Prompt(inv, cliui.PromptOptions{
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
					changes = append(changes, "Use new options")
				}
				// Only print when prompts are shown.
				if yes, _ := inv.ParsedFlags().GetBool("yes"); !yes {
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

			coderdConfig, err := client.SSHConfiguration(ctx)
			if err != nil {
				// If the error is 404, this deployment does not support
				// this endpoint yet. Do not error, just assume defaults.
				// TODO: Remove this in 2 months (May 31, 2023). Just return the error
				// 	and remove this 404 check.
				var sdkErr *codersdk.Error
				if !(xerrors.As(err, &sdkErr) && sdkErr.StatusCode() == http.StatusNotFound) {
					return xerrors.Errorf("fetch coderd config failed: %w", err)
				}
				coderdConfig.HostnamePrefix = "coder."
			}

			if sshConfigOpts.userHostPrefix != "" {
				// Override with user flag.
				coderdConfig.HostnamePrefix = sshConfigOpts.userHostPrefix
			}
			
			if sshConfigOpts.userHostnameSuffix != "" {
				// Override with user flag.
				coderdConfig.HostnameSuffix = sshConfigOpts.userHostnameSuffix
			}

			// Write agent configuration.
			defaultOptions := []string{
				"ConnectTimeout=0",
				"StrictHostKeyChecking=no",
				// Without this, the "REMOTE HOST IDENTITY CHANGED"
				// message will appear.
				"UserKnownHostsFile=/dev/null",
				// This disables the "Warning: Permanently added 'hostname' (RSA) to the list of known hosts."
				// message from appearing on every SSH. This happens because we ignore the known hosts.
				"LogLevel ERROR",
			}

			if !skipProxyCommand {
				rootFlags := fmt.Sprintf("--global-config %s", escapedGlobalConfig)
				for _, h := range sshConfigOpts.header {
					rootFlags += fmt.Sprintf(" --header %q", h)
				}
				if sshConfigOpts.headerCommand != "" {
					rootFlags += fmt.Sprintf(" --header-command %q", sshConfigOpts.headerCommand)
				}

				flags := ""
				if sshConfigOpts.waitEnum != "auto" {
					flags += " --wait=" + sshConfigOpts.waitEnum
				}
				if sshConfigOpts.disableAutostart {
					flags += " --disable-autostart=true"
				}
				
				// Add either the suffix or prefix flag for SSH
				sshHostFlag := ""
				if coderdConfig.HostnameSuffix != "" {
					sshHostFlag = fmt.Sprintf(" --ssh-hostname-suffix %s", coderdConfig.HostnameSuffix)
				} else {
					sshHostFlag = fmt.Sprintf(" --ssh-host-prefix %s", coderdConfig.HostnamePrefix)
				}
				
				defaultOptions = append(defaultOptions, fmt.Sprintf(
					"ProxyCommand %s %s ssh --stdio%s%s %%h",
					escapedCoderBinary, rootFlags, flags, sshHostFlag,
				))
			}

			// Create a copy of the options so we can modify them.
			configOptions := sshConfigOpts
			configOptions.sshOptions = nil

			// User options first (SSH only uses the first
			// option unless it can be given multiple times)
			for _, opt := range sshConfigOpts.sshOptions {
				err := configOptions.addOptions(opt)
				if err != nil {
					return xerrors.Errorf("add flag config option %q: %w", opt, err)
				}
			}

			// Deployment options second, allow them to
			// override standard options.
			for k, v := range coderdConfig.SSHConfigOptions {
				opt := fmt.Sprintf("%s %s", k, v)
				err := configOptions.addOptions(opt)
				if err != nil {
					return xerrors.Errorf("add coderd config option %q: %w", opt, err)
				}
			}

			// Finally, add the standard options.
			if err := configOptions.addOptions(defaultOptions...); err != nil {
				return err
			}

			// If we have a suffix, use that instead of the prefix
			var hostPattern string
			if coderdConfig.HostnameSuffix != "" {
				hostPattern = "Host *." + coderdConfig.HostnameSuffix
			} else {
				// Fall back to the old prefix-based pattern
				hostPattern = "Host " + coderdConfig.HostnamePrefix + "*"
			}
			hostBlock := []string{
				hostPattern,
			}
			// Prefix with '\t'
			for _, v := range configOptions.sshOptions {
				hostBlock = append(hostBlock, "\t"+v)
			}

			_, _ = buf.WriteString(strings.Join(hostBlock, "\n"))
			_ = buf.WriteByte('\n')

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

				color := isTTYOut(inv)
				diff, err := diffBytes(sshConfigFile, configRaw, configModified, color)
				if err != nil {
					return xerrors.Errorf("diff failed: %w", err)
				}
				if len(diff) > 0 {
					// Write diff to stdout.
					_, _ = fmt.Fprintf(inv.Stdout, "%s", diff)
				}

				return nil
			}

			if len(changes) > 0 {
				_, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:      fmt.Sprintf("The following changes will be made to your SSH configuration:\n\n    * %s\n\n  Continue?", strings.Join(changes, "\n    * ")),
					IsConfirm: true,
				})
				if err != nil {
					return nil
				}
				// Only print when prompts are shown.
				if yes, _ := inv.ParsedFlags().GetBool("yes"); !yes {
					_, _ = fmt.Fprint(out, "\n")
				}
			}

			if !bytes.Equal(configRaw, configModified) {
				err = atomic.WriteFile(sshConfigFile, bytes.NewReader(configModified))
				if err != nil {
					return xerrors.Errorf("write ssh config failed: %w", err)
				}
				_, _ = fmt.Fprintf(out, "Updated %q\n", sshConfigFile)
			}

			res, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{
				Owner: codersdk.Me,
				Limit: 1,
			})
			if err != nil {
				return xerrors.Errorf("fetch workspaces failed: %w", err)
			}

			if len(res.Workspaces) > 0 {
				_, _ = fmt.Fprintln(out, "You should now be able to ssh into your workspace.")
				var exampleCmd string
				if coderdConfig.HostnameSuffix != "" {
					exampleCmd = fmt.Sprintf("$ ssh %s.%s", res.Workspaces[0].Name, coderdConfig.HostnameSuffix)
				} else {
					exampleCmd = fmt.Sprintf("$ ssh %s%s", coderdConfig.HostnamePrefix, res.Workspaces[0].Name)
				}
				_, _ = fmt.Fprintf(out, "For example, try running:\n\n\t%s\n", exampleCmd)
			} else {
				_, _ = fmt.Fprint(out, "You don't have any workspaces yet, try creating one with:\n\n\t$ coder create <workspace>\n")
			}
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "ssh-config-file",
			Env:         "CODER_SSH_CONFIG_FILE",
			Default:     sshDefaultConfigFileName,
			Description: "Specifies the path to an SSH config.",
			Value:       serpent.StringOf(&sshConfigFile),
		},
		{
			Flag:    "coder-binary-path",
			Env:     "CODER_SSH_CONFIG_BINARY_PATH",
			Default: "",
			Description: "Optionally specify the absolute path to the coder binary used in ProxyCommand. " +
				"By default, the binary invoking this command ('config ssh') is used.",
			Value: serpent.Validate(serpent.StringOf(&coderCliPath), func(value *serpent.String) error {
				if runtime.GOOS == goosWindows {
					// For some reason filepath.IsAbs() does not work on windows.
					return nil
				}
				absolute := filepath.IsAbs(value.String())
				if !absolute {
					return xerrors.Errorf("coder cli path must be an absolute path")
				}
				return nil
			}),
		},
		{
			Flag:          "ssh-option",
			FlagShorthand: "o",
			Env:           "CODER_SSH_CONFIG_OPTS",
			Description:   "Specifies additional SSH options to embed in each host stanza.",
			Value:         serpent.StringArrayOf(&sshConfigOpts.sshOptions),
		},
		{
			Flag:          "dry-run",
			FlagShorthand: "n",
			Env:           "CODER_SSH_DRY_RUN",
			Description:   "Perform a trial run with no changes made, showing a diff at the end.",
			Value:         serpent.BoolOf(&dryRun),
		},
		{
			Flag:        "skip-proxy-command",
			Env:         "CODER_SSH_SKIP_PROXY_COMMAND",
			Description: "Specifies whether the ProxyCommand option should be skipped. Useful for testing.",
			Value:       serpent.BoolOf(&skipProxyCommand),
			Hidden:      true,
		},
		{
			Flag:        "use-previous-options",
			Env:         "CODER_SSH_USE_PREVIOUS_OPTIONS",
			Description: "Specifies whether or not to keep options from previous run of config-ssh.",
			Value:       serpent.BoolOf(&usePreviousOpts),
		},
		{
			Flag:        "ssh-host-prefix",
			Env:         "CODER_CONFIGSSH_SSH_HOST_PREFIX",
			Description: "Override the default host prefix.\nDEPRECATED: Use --ssh-hostname-suffix instead.",
			Value:       serpent.StringOf(&sshConfigOpts.userHostPrefix),
		},
		{
			Flag:        "ssh-hostname-suffix",
			Env:         "CODER_CONFIGSSH_SSH_HOSTNAME_SUFFIX",
			Description: "Override the default hostname suffix for workspace domain names.",
			Value:       serpent.StringOf(&sshConfigOpts.userHostnameSuffix),
		},
		{
			Flag:        "wait",
			Env:         "CODER_CONFIGSSH_WAIT", // Not to be mixed with CODER_SSH_WAIT.
			Description: "Specifies whether or not to wait for the startup script to finish executing. Auto means that the agent startup script behavior configured in the workspace template is used.",
			Default:     "auto",
			Value:       serpent.EnumOf(&sshConfigOpts.waitEnum, "yes", "no", "auto"),
		},
		{
			Flag:        "disable-autostart",
			Description: "Disable starting the workspace automatically when connecting via SSH.",
			Env:         "CODER_CONFIGSSH_DISABLE_AUTOSTART",
			Value:       serpent.BoolOf(&sshConfigOpts.disableAutostart),
			Default:     "false",
		},
		{
			Flag: "force-unix-filepaths",
			Env:  "CODER_CONFIGSSH_UNIX_FILEPATHS",
			Description: "By default, 'config-ssh' uses the os path separator when writing the ssh config. " +
				"This might be an issue in Windows machine that use a unix-like shell. " +
				"This flag forces the use of unix file paths (the forward slash '/').",
			Value: serpent.BoolOf(&forceUnixSeparators),
			// On non-windows showing this command is useless because it is a noop.
			// Hide vs disable it though so if a command is copied from a Windows
			// machine to a unix machine it will still work and not throw an
			// "unknown flag" error.
			Hidden: hideForceUnixSlashes,
		},
		cliui.SkipPromptOption(),
	}

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

	var ow strings.Builder
	if o.waitEnum != "auto" {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "wait", o.waitEnum)
	}
	if o.userHostPrefix != "" {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "ssh-host-prefix", o.userHostPrefix)
	}
	if o.userHostnameSuffix != "" {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "ssh-hostname-suffix", o.userHostnameSuffix)
	}
	if o.disableAutostart {
		_, _ = fmt.Fprintf(&ow, "# :%s=%v\n", "disable-autostart", o.disableAutostart)
	}
	for _, opt := range o.sshOptions {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "ssh-option", opt)
	}
	for _, h := range o.header {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "header", h)
	}
	if o.headerCommand != "" {
		_, _ = fmt.Fprintf(&ow, "# :%s=%s\n", "header-command", o.headerCommand)
	}
	if ow.Len() > 0 {
		_, _ = fmt.Fprint(w, sshConfigOptionsHeader)
		_, _ = fmt.Fprint(w, ow.String())
	}

	_, _ = fmt.Fprint(w, "#\n")
}

func sshConfigWriteSectionEnd(w io.Writer) {
	_, _ = fmt.Fprint(w, sshEndToken+"\n")
}

func sshConfigParseLastOptions(r io.Reader) (o sshConfigOptions) {
	// Default values.
	o.waitEnum = "auto"

	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		if strings.HasPrefix(line, "# :") {
			line = strings.TrimPrefix(line, "# :")
			parts := strings.SplitN(line, "=", 2)
			switch parts[0] {
			case "wait":
				o.waitEnum = parts[1]
			case "ssh-host-prefix":
				o.userHostPrefix = parts[1]
			case "ssh-hostname-suffix":
				o.userHostnameSuffix = parts[1]
			case "ssh-option":
				o.sshOptions = append(o.sshOptions, parts[1])
			case "disable-autostart":
				o.disableAutostart, _ = strconv.ParseBool(parts[1])
			case "header":
				o.header = append(o.header, parts[1])
			case "header-command":
				o.headerCommand = parts[1]
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
//
// Additional Windows-specific notes:
//
// In some situations a Windows user could be using a unix-like shell such as
// git bash. In these situations the coder.exe is using the windows filepath
// separator (\), but the shell wants the unix filepath separator (/).
// Trying to determine if the shell is unix-like is difficult, so this function
// takes the argument 'forceUnixPath' to force the filepath to be unix-like.
//
// On actual unix machines, this is **always** a noop. Even if a windows
// path is provided.
//
// Passing a "false" for forceUnixPath will result in the filepath separator
// untouched from the original input.
// ---
// This is a control flag, and that is ok. It is a control flag
// based on the OS of the user. Making this a different file is excessive.
// nolint:revive
func sshConfigExecEscape(path string, forceUnixPath bool) (string, error) {
	if forceUnixPath {
		// This is a workaround for #7639, where the filepath separator is
		// incorrectly the Windows separator (\) instead of the unix separator (/).
		path = filepath.ToSlash(path)
	}

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