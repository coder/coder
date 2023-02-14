package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/template"
	"time"

	"golang.org/x/xerrors"

	"github.com/charmbracelet/lipgloss"
	"github.com/kirsle/configdir"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/cliflag"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/config"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/codersdk/agentsdk"
)

var (
	Caret = cliui.Styles.Prompt.String()

	// Applied as annotations to workspace commands
	// so they display in a separated "help" section.
	workspaceCommand = map[string]string{
		"workspaces": "",
	}
)

const (
	varURL              = "url"
	varToken            = "token"
	varAgentToken       = "agent-token"
	varAgentURL         = "agent-url"
	varHeader           = "header"
	varNoOpen           = "no-open"
	varNoVersionCheck   = "no-version-warning"
	varNoFeatureWarning = "no-feature-warning"
	varForceTty         = "force-tty"
	varVerbose          = "verbose"
	notLoggedInMessage  = "You are not logged in. Try logging in using 'coder login <url>'."

	envNoVersionCheck   = "CODER_NO_VERSION_WARNING"
	envNoFeatureWarning = "CODER_NO_FEATURE_WARNING"
	envSessionToken     = "CODER_SESSION_TOKEN"
	envURL              = "CODER_URL"
)

var (
	errUnauthenticated = xerrors.New(notLoggedInMessage)
)

func init() {
	// Set cobra template functions in init to avoid conflicts in tests.
	cobra.AddTemplateFuncs(templateFunctions)
}

func Core() []*cobra.Command {
	// Please re-sort this list alphabetically if you change it!
	return []*cobra.Command{
		configSSH(),
		create(),
		deleteWorkspace(),
		dotfiles(),
		gitssh(),
		list(),
		login(),
		logout(),
		parameters(),
		ping(),
		portForward(),
		publickey(),
		rename(),
		resetPassword(),
		restart(),
		scaletest(),
		schedules(),
		show(),
		speedtest(),
		ssh(),
		start(),
		state(),
		stop(),
		templates(),
		tokens(),
		update(),
		users(),
		versionCmd(),
		vscodeSSH(),
		workspaceAgent(),
	}
}

func AGPL() []*cobra.Command {
	all := append(Core(), Server(deployment.NewViper(), func(_ context.Context, o *coderd.Options) (*coderd.API, io.Closer, error) {
		api := coderd.New(o)
		return api, api, nil
	}))
	return all
}

func Root(subcommands []*cobra.Command) *cobra.Command {
	// The GIT_ASKPASS environment variable must point at
	// a binary with no arguments. To prevent writing
	// cross-platform scripts to invoke the Coder binary
	// with a `gitaskpass` subcommand, we override the entrypoint
	// to check if the command was invoked.
	isGitAskpass := false

	fmtLong := `Coder %s — A tool for provisioning self-hosted development environments with Terraform.
`
	cmd := &cobra.Command{
		Use:           "coder",
		SilenceErrors: true,
		SilenceUsage:  true,
		Long:          fmt.Sprintf(fmtLong, buildinfo.Version()),
		Args: func(cmd *cobra.Command, args []string) error {
			if gitauth.CheckCommand(args, os.Environ()) {
				isGitAskpass = true
				return nil
			}
			return cobra.NoArgs(cmd, args)
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if isGitAskpass {
				return gitAskpass().RunE(cmd, args)
			}
			return cmd.Help()
		},
		Example: formatExamples(
			example{
				Description: "Start a Coder server",
				Command:     "coder server",
			},
			example{
				Description: "Get started by creating a template from an example",
				Command:     "coder templates init",
			},
		),
	}

	cmd.AddCommand(subcommands...)
	fixUnknownSubcommandError(cmd.Commands())

	cmd.SetUsageTemplate(usageTemplate())

	cliflag.String(cmd.PersistentFlags(), varURL, "", envURL, "", "URL to a deployment.")
	cliflag.Bool(cmd.PersistentFlags(), varNoVersionCheck, "", envNoVersionCheck, false, "Suppress warning when client and server versions do not match.")
	cliflag.Bool(cmd.PersistentFlags(), varNoFeatureWarning, "", envNoFeatureWarning, false, "Suppress warnings about unlicensed features.")
	cliflag.String(cmd.PersistentFlags(), varToken, "", envSessionToken, "", fmt.Sprintf("Specify an authentication token. For security reasons setting %s is preferred.", envSessionToken))
	cliflag.String(cmd.PersistentFlags(), varAgentToken, "", "CODER_AGENT_TOKEN", "", "An agent authentication token.")
	_ = cmd.PersistentFlags().MarkHidden(varAgentToken)
	cliflag.String(cmd.PersistentFlags(), varAgentURL, "", "CODER_AGENT_URL", "", "URL for an agent to access your deployment.")
	_ = cmd.PersistentFlags().MarkHidden(varAgentURL)
	cliflag.String(cmd.PersistentFlags(), config.FlagName, "", "CODER_CONFIG_DIR", configdir.LocalConfig("coderv2"), "Path to the global `coder` config directory.")
	cliflag.StringArray(cmd.PersistentFlags(), varHeader, "", "CODER_HEADER", []string{}, "HTTP headers added to all requests. Provide as \"Key=Value\"")
	cmd.PersistentFlags().Bool(varForceTty, false, "Force the `coder` command to run as if connected to a TTY.")
	_ = cmd.PersistentFlags().MarkHidden(varForceTty)
	cmd.PersistentFlags().Bool(varNoOpen, false, "Block automatically opening URLs in the browser.")
	_ = cmd.PersistentFlags().MarkHidden(varNoOpen)
	cliflag.Bool(cmd.PersistentFlags(), varVerbose, "v", "CODER_VERBOSE", false, "Enable verbose output.")

	return cmd
}

// fixUnknownSubcommandError modifies the provided commands so that the
// ones with subcommands output the correct error message when an
// unknown subcommand is invoked.
//
// Example:
//
//	unknown command "bad" for "coder templates"
func fixUnknownSubcommandError(commands []*cobra.Command) {
	for _, sc := range commands {
		if sc.HasSubCommands() {
			if sc.Run == nil && sc.RunE == nil {
				if sc.Args != nil {
					// In case the developer does not know about this
					// behavior in Cobra they must verify correct
					// behavior. For instance, settings Args to
					// `cobra.ExactArgs(0)` will not give the same
					// message as `cobra.NoArgs`. Likewise, omitting the
					// run function will not give the wanted error.
					panic("developer error: subcommand has subcommands and Args but no Run or RunE")
				}
				sc.Args = cobra.NoArgs
				sc.Run = func(*cobra.Command, []string) {}
			}

			fixUnknownSubcommandError(sc.Commands())
		}
	}
}

// versionCmd prints the coder version
func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show coder version",
		RunE: func(cmd *cobra.Command, args []string) error {
			var str strings.Builder
			_, _ = str.WriteString("Coder ")
			if buildinfo.IsAGPL() {
				_, _ = str.WriteString("(AGPL) ")
			}
			_, _ = str.WriteString(buildinfo.Version())
			buildTime, valid := buildinfo.Time()
			if valid {
				_, _ = str.WriteString(" " + buildTime.Format(time.UnixDate))
			}
			_, _ = str.WriteString("\r\n" + buildinfo.ExternalURL() + "\r\n\r\n")

			if buildinfo.IsSlim() {
				_, _ = str.WriteString(fmt.Sprintf("Slim build of Coder, does not support the %s subcommand.\n", cliui.Styles.Code.Render("server")))
			} else {
				_, _ = str.WriteString(fmt.Sprintf("Full build of Coder, supports the %s subcommand.\n", cliui.Styles.Code.Render("server")))
			}

			_, _ = fmt.Fprint(cmd.OutOrStdout(), str.String())
			return nil
		},
	}
}

func isTest() bool {
	return flag.Lookup("test.v") != nil
}

// CreateClient returns a new client from the command context.
// It reads from global configuration files if flags are not set.
func CreateClient(cmd *cobra.Command) (*codersdk.Client, error) {
	root := createConfig(cmd)
	rawURL, err := cmd.Flags().GetString(varURL)
	if err != nil || rawURL == "" {
		rawURL, err = root.URL().Read()
		if err != nil {
			// If the configuration files are absent, the user is logged out
			if os.IsNotExist(err) {
				return nil, errUnauthenticated
			}
			return nil, err
		}
	}
	serverURL, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, err
	}
	token, err := cmd.Flags().GetString(varToken)
	if err != nil || token == "" {
		token, err = root.Session().Read()
		if err != nil {
			// If the configuration files are absent, the user is logged out
			if os.IsNotExist(err) {
				return nil, errUnauthenticated
			}
			return nil, err
		}
	}
	client, err := createUnauthenticatedClient(cmd, serverURL)
	if err != nil {
		return nil, err
	}
	client.SetSessionToken(token)

	// We send these requests in parallel to minimize latency.
	var (
		versionErr = make(chan error)
		warningErr = make(chan error)
	)
	go func() {
		versionErr <- checkVersions(cmd, client)
		close(versionErr)
	}()

	go func() {
		warningErr <- checkWarnings(cmd, client)
		close(warningErr)
	}()

	if err = <-versionErr; err != nil {
		// Just log the error here. We never want to fail a command
		// due to a pre-run.
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			cliui.Styles.Warn.Render("check versions error: %s"), err)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	}

	if err = <-warningErr; err != nil {
		// Same as above
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			cliui.Styles.Warn.Render("check entitlement warnings error: %s"), err)
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	}

	return client, nil
}

func createUnauthenticatedClient(cmd *cobra.Command, serverURL *url.URL) (*codersdk.Client, error) {
	client := codersdk.New(serverURL)
	headers, err := cmd.Flags().GetStringArray(varHeader)
	if err != nil {
		return nil, err
	}
	transport := &headerTransport{
		transport: http.DefaultTransport,
		headers:   map[string]string{},
	}
	for _, header := range headers {
		parts := strings.SplitN(header, "=", 2)
		if len(parts) < 2 {
			return nil, xerrors.Errorf("split header %q had less than two parts", header)
		}
		transport.headers[parts[0]] = parts[1]
	}
	client.HTTPClient.Transport = transport
	return client, nil
}

// createAgentClient returns a new client from the command context.
// It works just like CreateClient, but uses the agent token and URL instead.
func createAgentClient(cmd *cobra.Command) (*agentsdk.Client, error) {
	rawURL, err := cmd.Flags().GetString(varAgentURL)
	if err != nil {
		return nil, err
	}
	serverURL, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	token, err := cmd.Flags().GetString(varAgentToken)
	if err != nil {
		return nil, err
	}
	client := agentsdk.New(serverURL)
	client.SetSessionToken(token)
	return client, nil
}

// CurrentOrganization returns the currently active organization for the authenticated user.
func CurrentOrganization(cmd *cobra.Command, client *codersdk.Client) (codersdk.Organization, error) {
	orgs, err := client.OrganizationsByUser(cmd.Context(), codersdk.Me)
	if err != nil {
		return codersdk.Organization{}, nil
	}
	// For now, we won't use the config to set this.
	// Eventually, we will support changing using "coder switch <org>"
	return orgs[0], nil
}

// namedWorkspace fetches and returns a workspace by an identifier, which may be either
// a bare name (for a workspace owned by the current user) or a "user/workspace" combination,
// where user is either a username or UUID.
func namedWorkspace(cmd *cobra.Command, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	parts := strings.Split(identifier, "/")

	var owner, name string
	switch len(parts) {
	case 1:
		owner = codersdk.Me
		name = parts[0]
	case 2:
		owner = parts[0]
		name = parts[1]
	default:
		return codersdk.Workspace{}, xerrors.Errorf("invalid workspace name: %q", identifier)
	}

	return client.WorkspaceByOwnerAndName(cmd.Context(), owner, name, codersdk.WorkspaceOptions{})
}

// createConfig consumes the global configuration flag to produce a config root.
func createConfig(cmd *cobra.Command) config.Root {
	globalRoot, err := cmd.Flags().GetString(config.FlagName)
	if err != nil {
		panic(err)
	}
	return config.Root(globalRoot)
}

// isTTY returns whether the passed reader is a TTY or not.
// This accepts a reader to work with Cobra's "InOrStdin"
// function for simple testing.
func isTTY(cmd *cobra.Command) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := cmd.Flags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := cmd.InOrStdin().(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

// isTTYOut returns whether the passed reader is a TTY or not.
// This accepts a reader to work with Cobra's "OutOrStdout"
// function for simple testing.
func isTTYOut(cmd *cobra.Command) bool {
	return isTTYWriter(cmd, cmd.OutOrStdout)
}

// isTTYErr returns whether the passed reader is a TTY or not.
// This accepts a reader to work with Cobra's "ErrOrStderr"
// function for simple testing.
func isTTYErr(cmd *cobra.Command) bool {
	return isTTYWriter(cmd, cmd.ErrOrStderr)
}

func isTTYWriter(cmd *cobra.Command, writer func() io.Writer) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := cmd.Flags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := writer().(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

var templateFunctions = template.FuncMap{
	"usageHeader":        usageHeader,
	"isWorkspaceCommand": isWorkspaceCommand,
}

func usageHeader(s string) string {
	// Customizes the color of headings to make subcommands more visually
	// appealing.
	return cliui.Styles.Placeholder.Render(s)
}

func isWorkspaceCommand(cmd *cobra.Command) bool {
	if _, ok := cmd.Annotations["workspaces"]; ok {
		return true
	}
	var ws bool
	cmd.VisitParents(func(cmd *cobra.Command) {
		if _, ok := cmd.Annotations["workspaces"]; ok {
			ws = true
		}
	})
	return ws
}

func usageTemplate() string {
	// usageHeader is defined in init().
	return `{{usageHeader "Usage:"}}
{{- if .Runnable}}
  {{.UseLine}}
{{end}}
{{- if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]
{{end}}

{{- if gt (len .Aliases) 0}}
{{usageHeader "Aliases:"}}
  {{.NameAndAliases}}
{{end}}

{{- if .HasExample}}
{{usageHeader "Get Started:"}}
{{.Example}}
{{end}}

{{- $isRootHelp := (not .HasParent)}}
{{- if .HasAvailableSubCommands}}
{{usageHeader "Commands:"}}
  {{- range .Commands}}
    {{- $isRootWorkspaceCommand := (and $isRootHelp (isWorkspaceCommand .))}}
    {{- if (or (and .IsAvailableCommand (not $isRootWorkspaceCommand)) (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if (and $isRootHelp .HasAvailableSubCommands)}}
{{usageHeader "Workspace Commands:"}}
  {{- range .Commands}}
    {{- if (and .IsAvailableCommand (isWorkspaceCommand .))}}
  {{rpad .Name .NamePadding }} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableLocalFlags}}
{{usageHeader "Flags:"}}
{{.LocalFlags.FlagUsagesWrapped 100 | trimTrailingWhitespaces}}
{{end}}

{{- if .HasAvailableInheritedFlags}}
{{usageHeader "Global Flags:"}}
{{.InheritedFlags.FlagUsagesWrapped 100 | trimTrailingWhitespaces}}
{{end}}

{{- if .HasHelpSubCommands}}
{{usageHeader "Additional help topics:"}}
  {{- range .Commands}}
    {{- if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}
    {{- end}}
  {{- end}}
{{end}}

{{- if .HasAvailableSubCommands}}
Use "{{.CommandPath}} [command] --help" for more information about a command.
{{end}}`
}

// example represents a standard example for command usage, to be used
// with formatExamples.
type example struct {
	Description string
	Command     string
}

// formatExamples formats the examples as width wrapped bulletpoint
// descriptions with the command underneath.
func formatExamples(examples ...example) string {
	wrap := cliui.Styles.Wrap.Copy()
	wrap.PaddingLeft(4)
	var sb strings.Builder
	for i, e := range examples {
		if len(e.Description) > 0 {
			_, _ = sb.WriteString("  - " + wrap.Render(e.Description + ":")[4:] + "\n\n    ")
		}
		// We add 1 space here because `cliui.Styles.Code` adds an extra
		// space. This makes the code block align at an even 2 or 6
		// spaces for symmetry.
		_, _ = sb.WriteString(" " + cliui.Styles.Code.Render(fmt.Sprintf("$ %s", e.Command)))
		if i < len(examples)-1 {
			_, _ = sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

// FormatCobraError colorizes and adds "--help" docs to cobra commands.
func FormatCobraError(err error, cmd *cobra.Command) string {
	helpErrMsg := fmt.Sprintf("Run '%s --help' for usage.", cmd.CommandPath())

	var (
		httpErr *codersdk.Error
		output  strings.Builder
	)

	if xerrors.As(err, &httpErr) {
		_, _ = fmt.Fprintln(&output, httpErr.Friendly())
	}

	// If the httpErr is nil then we just have a regular error in which
	// case we want to print out what's happening.
	if httpErr == nil || cliflag.IsSetBool(cmd, varVerbose) {
		_, _ = fmt.Fprintln(&output, err.Error())
	}

	_, _ = fmt.Fprint(&output, helpErrMsg)

	return cliui.Styles.Error.Render(output.String())
}

func checkVersions(cmd *cobra.Command, client *codersdk.Client) error {
	if cliflag.IsSetBool(cmd, varNoVersionCheck) {
		return nil
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	clientVersion := buildinfo.Version()
	info, err := client.BuildInfo(ctx)
	// Avoid printing errors that are connection-related.
	if isConnectionError(err) {
		return nil
	}

	if err != nil {
		return xerrors.Errorf("build info: %w", err)
	}

	fmtWarningText := `version mismatch: client %s, server %s
`
	// Our installation script doesn't work on Windows, so instead we direct the user
	// to the GitHub release page to download the latest installer.
	if runtime.GOOS == "windows" {
		fmtWarningText += `download the server version from: https://github.com/coder/coder/releases/v%s`
	} else {
		fmtWarningText += `download the server version with: 'curl -L https://coder.com/install.sh | sh -s -- --version %s'`
	}

	if !buildinfo.VersionsMatch(clientVersion, info.Version) {
		warn := cliui.Styles.Warn.Copy().Align(lipgloss.Left)
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), warn.Render(fmtWarningText), clientVersion, info.Version, strings.TrimPrefix(info.CanonicalVersion(), "v"))
		_, _ = fmt.Fprintln(cmd.ErrOrStderr())
	}

	return nil
}

func checkWarnings(cmd *cobra.Command, client *codersdk.Client) error {
	if cliflag.IsSetBool(cmd, varNoFeatureWarning) {
		return nil
	}

	ctx, cancel := context.WithTimeout(cmd.Context(), 10*time.Second)
	defer cancel()

	entitlements, err := client.Entitlements(ctx)
	if err == nil {
		for _, w := range entitlements.Warnings {
			_, _ = fmt.Fprintln(cmd.ErrOrStderr(), cliui.Styles.Warn.Render(w))
		}
	}
	return nil
}

type headerTransport struct {
	transport http.RoundTripper
	headers   map[string]string
}

func (h *headerTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range h.headers {
		req.Header.Add(k, v)
	}
	return h.transport.RoundTrip(req)
}

// dumpHandler provides a custom SIGQUIT and SIGTRAP handler that dumps the
// stacktrace of all goroutines to stderr and a well-known file in the home
// directory. This is useful for debugging deadlock issues that may occur in
// production in workspaces, since the default Go runtime will only dump to
// stderr (which is often difficult/impossible to read in a workspace).
//
// SIGQUITs will still cause the program to exit (similarly to the default Go
// runtime behavior).
//
// A SIGQUIT handler will not be registered if GOTRACEBACK=crash.
//
// On Windows this immediately returns.
func dumpHandler(ctx context.Context) {
	if runtime.GOOS == "windows" {
		// free up the goroutine since it'll be permanently blocked anyways
		return
	}

	listenSignals := []os.Signal{syscall.SIGTRAP}
	if os.Getenv("GOTRACEBACK") != "crash" {
		listenSignals = append(listenSignals, syscall.SIGQUIT)
	}

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, listenSignals...)
	defer signal.Stop(sigs)

	for {
		sigStr := ""
		select {
		case <-ctx.Done():
			return
		case sig := <-sigs:
			switch sig {
			case syscall.SIGQUIT:
				sigStr = "SIGQUIT"
			case syscall.SIGTRAP:
				sigStr = "SIGTRAP"
			}
		}

		// Start with a 1MB buffer and keep doubling it until we can fit the
		// entire stacktrace, stopping early once we reach 64MB.
		buf := make([]byte, 1_000_000)
		stacklen := 0
		for {
			stacklen = runtime.Stack(buf, true)
			if stacklen < len(buf) {
				break
			}
			if 2*len(buf) > 64_000_000 {
				// Write a message to the end of the buffer saying that it was
				// truncated.
				const truncatedMsg = "\n\n\nstack trace truncated due to size\n"
				copy(buf[len(buf)-len(truncatedMsg):], truncatedMsg)
				break
			}
			buf = make([]byte, 2*len(buf))
		}

		_, _ = fmt.Fprintf(os.Stderr, "%s:\n%s\n", sigStr, buf[:stacklen])

		// Write to a well-known file.
		dir, err := os.UserHomeDir()
		if err != nil {
			dir = os.TempDir()
		}
		fpath := filepath.Join(dir, fmt.Sprintf("coder-agent-%s.dump", time.Now().Format("2006-01-02T15:04:05.000Z")))
		_, _ = fmt.Fprintf(os.Stderr, "writing dump to %q\n", fpath)

		f, err := os.Create(fpath)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to open dump file: %v\n", err.Error())
			goto done
		}
		_, err = f.Write(buf[:stacklen])
		_ = f.Close()
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "failed to write dump file: %v\n", err.Error())
			goto done
		}

	done:
		if sigStr == "SIGQUIT" {
			//nolint:revive
			os.Exit(1)
		}
	}
}

// IiConnectionErr is a convenience function for checking if the source of an
// error is due to a 'connection refused', 'no such host', etc.
func isConnectionError(err error) bool {
	var (
		// E.g. no such host
		dnsErr *net.DNSError
		// Eg. connection refused
		opErr *net.OpError
	)

	return xerrors.As(err, &dnsErr) || xerrors.As(err, &opErr)
}
