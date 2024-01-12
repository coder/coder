package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/mattn/go-isatty"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

var (
	Caret = pretty.Sprint(cliui.DefaultStyles.Prompt, "")

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
	varAgentTokenFile   = "agent-token-file"
	varAgentURL         = "agent-url"
	varHeader           = "header"
	varHeaderCommand    = "header-command"
	varNoOpen           = "no-open"
	varNoVersionCheck   = "no-version-warning"
	varNoFeatureWarning = "no-feature-warning"
	varForceTty         = "force-tty"
	varVerbose          = "verbose"
	varDisableDirect    = "disable-direct-connections"
	notLoggedInMessage  = "You are not logged in. Try logging in using 'coder login <url>'."

	envNoVersionCheck   = "CODER_NO_VERSION_WARNING"
	envNoFeatureWarning = "CODER_NO_FEATURE_WARNING"
	envSessionToken     = "CODER_SESSION_TOKEN"
	//nolint:gosec
	envAgentToken = "CODER_AGENT_TOKEN"
	//nolint:gosec
	envAgentTokenFile = "CODER_AGENT_TOKEN_FILE"
	envURL            = "CODER_URL"
)

var errUnauthenticated = xerrors.New(notLoggedInMessage)

func (r *RootCmd) Core() []*clibase.Cmd {
	// Please re-sort this list alphabetically if you change it!
	return []*clibase.Cmd{
		r.dotfiles(),
		r.externalAuth(),
		r.login(),
		r.logout(),
		r.netcheck(),
		r.portForward(),
		r.publickey(),
		r.resetPassword(),
		r.state(),
		r.templates(),
		r.tokens(),
		r.users(),
		r.version(defaultVersionInfo),

		// Workspace Commands
		r.autoupdate(),
		r.configSSH(),
		r.create(),
		r.deleteWorkspace(),
		r.list(),
		r.open(),
		r.ping(),
		r.rename(),
		r.restart(),
		r.schedules(),
		r.show(),
		r.speedtest(),
		r.ssh(),
		r.start(),
		r.stat(),
		r.stop(),
		r.update(),

		// Hidden
		r.gitssh(),
		r.vscodeSSH(),
		r.workspaceAgent(),
		r.expCmd(),
	}
}

func (r *RootCmd) AGPL() []*clibase.Cmd {
	all := append(r.Core(), r.Server( /* Do not import coderd here. */ nil))
	return all
}

// Main is the entrypoint for the Coder CLI.
func (r *RootCmd) RunMain(subcommands []*clibase.Cmd) {
	rand.Seed(time.Now().UnixMicro())

	cmd, err := r.Command(subcommands)
	if err != nil {
		panic(err)
	}
	err = cmd.Invoke().WithOS().Run()
	if err != nil {
		code := 1
		var exitErr *exitError
		if errors.As(err, &exitErr) {
			code = exitErr.code
			err = exitErr.err
		}
		if errors.Is(err, cliui.Canceled) {
			//nolint:revive
			os.Exit(code)
		}
		f := prettyErrorFormatter{w: os.Stderr, verbose: r.verbose}
		if err != nil {
			f.format(err)
		}
		//nolint:revive
		os.Exit(code)
	}
}

func (r *RootCmd) Command(subcommands []*clibase.Cmd) (*clibase.Cmd, error) {
	fmtLong := `Coder %s — A tool for provisioning self-hosted development environments with Terraform.
`
	cmd := &clibase.Cmd{
		Use: "coder [global-flags] <subcommand>",
		Long: fmt.Sprintf(fmtLong, buildinfo.Version()) + formatExamples(
			example{
				Description: "Start a Coder server",
				Command:     "coder server",
			},
			example{
				Description: "Get started by creating a template from an example",
				Command:     "coder templates init",
			},
		),
		Handler: func(i *clibase.Invocation) error {
			if r.versionFlag {
				return r.version(defaultVersionInfo).Handler(i)
			}
			// The GIT_ASKPASS environment variable must point at
			// a binary with no arguments. To prevent writing
			// cross-platform scripts to invoke the Coder binary
			// with a `gitaskpass` subcommand, we override the entrypoint
			// to check if the command was invoked.
			if gitauth.CheckCommand(i.Args, i.Environ.ToOS()) {
				return r.gitAskpass().Handler(i)
			}
			return i.Command.HelpHandler(i)
		},
	}

	cmd.AddSubcommands(subcommands...)

	// Set default help handler for all commands.
	cmd.Walk(func(c *clibase.Cmd) {
		if c.HelpHandler == nil {
			c.HelpHandler = helpFn()
		}
	})

	var merr error
	// Add [flags] to usage when appropriate.
	cmd.Walk(func(cmd *clibase.Cmd) {
		const flags = "[flags]"
		if strings.Contains(cmd.Use, flags) {
			merr = errors.Join(
				merr,
				xerrors.Errorf(
					"command %q shouldn't have %q in usage since it's automatically populated",
					cmd.FullUsage(),
					flags,
				),
			)
			return
		}

		var hasFlag bool
		for _, opt := range cmd.Options {
			if opt.Flag != "" {
				hasFlag = true
				break
			}
		}

		if !hasFlag {
			return
		}

		// We insert [flags] between the command's name and its arguments.
		tokens := strings.SplitN(cmd.Use, " ", 2)
		if len(tokens) == 1 {
			cmd.Use = fmt.Sprintf("%s %s", tokens[0], flags)
			return
		}
		cmd.Use = fmt.Sprintf("%s %s %s", tokens[0], flags, tokens[1])
	})

	// Add alises when appropriate.
	cmd.Walk(func(cmd *clibase.Cmd) {
		// TODO: we should really be consistent about naming.
		if cmd.Name() == "delete" || cmd.Name() == "remove" {
			if slices.Contains(cmd.Aliases, "rm") {
				merr = errors.Join(
					merr,
					xerrors.Errorf("command %q shouldn't have alias %q since it's added automatically", cmd.FullName(), "rm"),
				)
				return
			}
			cmd.Aliases = append(cmd.Aliases, "rm")
		}
	})

	// Sanity-check command options.
	cmd.Walk(func(cmd *clibase.Cmd) {
		for _, opt := range cmd.Options {
			// Verify that every option is configurable.
			if opt.Flag == "" && opt.Env == "" {
				if cmd.Name() == "server" {
					// The server command is funky and has YAML-only options, e.g.
					// support links.
					return
				}
				merr = errors.Join(
					merr,
					xerrors.Errorf("option %q in %q should have a flag or env", opt.Name, cmd.FullName()),
				)
			}
		}
	})
	if merr != nil {
		return nil, merr
	}

	var debugOptions bool

	// Add a wrapper to every command to enable debugging options.
	cmd.Walk(func(cmd *clibase.Cmd) {
		h := cmd.Handler
		if h == nil {
			// We should never have a nil handler, but if we do, do not
			// wrap it. Wrapping it just hides a nil pointer dereference.
			// If a nil handler exists, this is a developer bug. If no handler
			// is required for a command such as command grouping (e.g. `users'
			// and 'groups'), then the handler should be set to the helper
			// function.
			//	func(inv *clibase.Invocation) error {
			//		return inv.Command.HelpHandler(inv)
			//	}
			return
		}
		cmd.Handler = func(i *clibase.Invocation) error {
			if !debugOptions {
				return h(i)
			}

			tw := tabwriter.NewWriter(i.Stdout, 0, 0, 4, ' ', 0)
			_, _ = fmt.Fprintf(tw, "Option\tValue Source\n")
			for _, opt := range cmd.Options {
				_, _ = fmt.Fprintf(tw, "%q\t%v\n", opt.Name, opt.ValueSource)
			}
			tw.Flush()
			return nil
		}
	})

	if r.agentURL == nil {
		r.agentURL = new(url.URL)
	}
	if r.clientURL == nil {
		r.clientURL = new(url.URL)
	}

	globalGroup := &clibase.Group{
		Name:        "Global",
		Description: `Global options are applied to all commands. They can be set using environment variables or flags.`,
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        varURL,
			Env:         envURL,
			Description: "URL to a deployment.",
			Value:       clibase.URLOf(r.clientURL),
			Group:       globalGroup,
		},
		{
			Flag:        "debug-options",
			Description: "Print all options, how they're set, then exit.",
			Value:       clibase.BoolOf(&debugOptions),
			Group:       globalGroup,
		},
		{
			Flag:        varToken,
			Env:         envSessionToken,
			Description: fmt.Sprintf("Specify an authentication token. For security reasons setting %s is preferred.", envSessionToken),
			Value:       clibase.StringOf(&r.token),
			Group:       globalGroup,
		},
		{
			Flag:        varAgentToken,
			Env:         envAgentToken,
			Description: "An agent authentication token.",
			Value:       clibase.StringOf(&r.agentToken),
			Hidden:      true,
			Group:       globalGroup,
		},
		{
			Flag:        varAgentTokenFile,
			Env:         envAgentTokenFile,
			Description: "A file containing an agent authentication token.",
			Value:       clibase.StringOf(&r.agentTokenFile),
			Hidden:      true,
			Group:       globalGroup,
		},
		{
			Flag:        varAgentURL,
			Env:         "CODER_AGENT_URL",
			Description: "URL for an agent to access your deployment.",
			Value:       clibase.URLOf(r.agentURL),
			Hidden:      true,
			Group:       globalGroup,
		},
		{
			Flag:        varNoVersionCheck,
			Env:         envNoVersionCheck,
			Description: "Suppress warning when client and server versions do not match.",
			Value:       clibase.BoolOf(&r.noVersionCheck),
			Group:       globalGroup,
		},
		{
			Flag:        varNoFeatureWarning,
			Env:         envNoFeatureWarning,
			Description: "Suppress warnings about unlicensed features.",
			Value:       clibase.BoolOf(&r.noFeatureWarning),
			Group:       globalGroup,
		},
		{
			Flag:        varHeader,
			Env:         "CODER_HEADER",
			Description: "Additional HTTP headers added to all requests. Provide as " + `key=value` + ". Can be specified multiple times.",
			Value:       clibase.StringArrayOf(&r.header),
			Group:       globalGroup,
		},
		{
			Flag:        varHeaderCommand,
			Env:         "CODER_HEADER_COMMAND",
			Description: "An external command that outputs additional HTTP headers added to all requests. The command must output each header as `key=value` on its own line.",
			Value:       clibase.StringOf(&r.headerCommand),
			Group:       globalGroup,
		},
		{
			Flag:        varNoOpen,
			Env:         "CODER_NO_OPEN",
			Description: "Suppress opening the browser after logging in.",
			Value:       clibase.BoolOf(&r.noOpen),
			Hidden:      true,
			Group:       globalGroup,
		},
		{
			Flag:        varForceTty,
			Env:         "CODER_FORCE_TTY",
			Hidden:      true,
			Description: "Force the use of a TTY.",
			Value:       clibase.BoolOf(&r.forceTTY),
			Group:       globalGroup,
		},
		{
			Flag:          varVerbose,
			FlagShorthand: "v",
			Env:           "CODER_VERBOSE",
			Description:   "Enable verbose output.",
			Value:         clibase.BoolOf(&r.verbose),
			Group:         globalGroup,
		},
		{
			Flag:        varDisableDirect,
			Env:         "CODER_DISABLE_DIRECT_CONNECTIONS",
			Description: "Disable direct (P2P) connections to workspaces.",
			Value:       clibase.BoolOf(&r.disableDirect),
			Group:       globalGroup,
		},
		{
			Flag:        "debug-http",
			Description: "Debug codersdk HTTP requests.",
			Value:       clibase.BoolOf(&r.debugHTTP),
			Group:       globalGroup,
			Hidden:      true,
		},
		{
			Flag:        config.FlagName,
			Env:         "CODER_CONFIG_DIR",
			Description: "Path to the global `coder` config directory.",
			Default:     config.DefaultDir(),
			Value:       clibase.StringOf(&r.globalConfig),
			Group:       globalGroup,
		},
		{
			Flag: "version",
			// This was requested by a customer to assist with their migration.
			// They have two Coder CLIs, and want to tell the difference by running
			// the same base command.
			Description: "Run the version command. Useful for v1 customers migrating to v2.",
			Value:       clibase.BoolOf(&r.versionFlag),
			Hidden:      true,
		},
	}

	err := cmd.PrepareAll()
	if err != nil {
		return nil, err
	}

	return cmd, nil
}

// RootCmd contains parameters and helpers useful to all commands.
type RootCmd struct {
	clientURL      *url.URL
	token          string
	globalConfig   string
	header         []string
	headerCommand  string
	agentToken     string
	agentTokenFile string
	agentURL       *url.URL
	forceTTY       bool
	noOpen         bool
	verbose        bool
	versionFlag    bool
	disableDirect  bool
	debugHTTP      bool

	noVersionCheck   bool
	noFeatureWarning bool
}

func addTelemetryHeader(client *codersdk.Client, inv *clibase.Invocation) {
	transport, ok := client.HTTPClient.Transport.(*codersdk.HeaderTransport)
	if !ok {
		transport = &codersdk.HeaderTransport{
			Transport: client.HTTPClient.Transport,
			Header:    http.Header{},
		}
		client.HTTPClient.Transport = transport
	}

	var topts []telemetry.Option
	for _, opt := range inv.Command.FullOptions() {
		if opt.ValueSource == clibase.ValueSourceNone || opt.ValueSource == clibase.ValueSourceDefault {
			continue
		}
		topts = append(topts, telemetry.Option{
			Name:        opt.Name,
			ValueSource: string(opt.ValueSource),
		})
	}
	ti := telemetry.Invocation{
		Command:   inv.Command.FullName(),
		Options:   topts,
		InvokedAt: time.Now(),
	}

	byt, err := json.Marshal(ti)
	if err != nil {
		// Should be impossible
		panic(err)
	}

	// Per https://stackoverflow.com/questions/686217/maximum-on-http-header-values,
	// we don't want to send headers that are too long.
	s := base64.StdEncoding.EncodeToString(byt)
	if len(s) > 4096 {
		return
	}

	transport.Header.Add(codersdk.CLITelemetryHeader, s)
}

// InitClient sets client to a new client.
// It reads from global configuration files if flags are not set.
func (r *RootCmd) InitClient(client *codersdk.Client) clibase.MiddlewareFunc {
	return clibase.Chain(
		r.initClientInternal(client, false),
		// By default, we should print warnings in addition to initializing the client
		r.PrintWarnings(client),
	)
}

func (r *RootCmd) InitClientMissingTokenOK(client *codersdk.Client) clibase.MiddlewareFunc {
	return r.initClientInternal(client, true)
}

// nolint: revive
func (r *RootCmd) initClientInternal(client *codersdk.Client, allowTokenMissing bool) clibase.MiddlewareFunc {
	if client == nil {
		panic("client is nil")
	}
	if r == nil {
		panic("root is nil")
	}
	return func(next clibase.HandlerFunc) clibase.HandlerFunc {
		return func(inv *clibase.Invocation) error {
			conf := r.createConfig()
			var err error
			if r.clientURL == nil || r.clientURL.String() == "" {
				rawURL, err := conf.URL().Read()
				// If the configuration files are absent, the user is logged out
				if os.IsNotExist(err) {
					return errUnauthenticated
				}
				if err != nil {
					return err
				}

				r.clientURL, err = url.Parse(strings.TrimSpace(rawURL))
				if err != nil {
					return err
				}
			}

			if r.token == "" {
				r.token, err = conf.Session().Read()
				// If the configuration files are absent, the user is logged out
				if os.IsNotExist(err) {
					if !allowTokenMissing {
						return errUnauthenticated
					}
				} else if err != nil {
					return err
				}
			}
			err = r.setClient(inv.Context(), client, r.clientURL)
			if err != nil {
				return err
			}

			addTelemetryHeader(client, inv)

			client.SetSessionToken(r.token)

			if r.debugHTTP {
				client.PlainLogger = os.Stderr
				client.SetLogBodies(true)
			}
			client.DisableDirectConnections = r.disableDirect
			return next(inv)
		}
	}
}

func (r *RootCmd) PrintWarnings(client *codersdk.Client) clibase.MiddlewareFunc {
	if client == nil {
		panic("client is nil")
	}
	if r == nil {
		panic("root is nil")
	}
	return func(next clibase.HandlerFunc) clibase.HandlerFunc {
		return func(inv *clibase.Invocation) error {
			// We send these requests in parallel to minimize latency.
			var (
				versionErr = make(chan error)
				warningErr = make(chan error)
			)
			go func() {
				versionErr <- r.checkVersions(inv, client)
				close(versionErr)
			}()

			go func() {
				warningErr <- r.checkWarnings(inv, client)
				close(warningErr)
			}()

			if err := <-versionErr; err != nil {
				// Just log the error here. We never want to fail a command
				// due to a pre-run.
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Warn, "check versions error: %s", err)
				_, _ = fmt.Fprintln(inv.Stderr)
			}

			if err := <-warningErr; err != nil {
				// Same as above
				pretty.Fprintf(inv.Stderr, cliui.DefaultStyles.Warn, "check entitlement warnings error: %s", err)
				_, _ = fmt.Fprintln(inv.Stderr)
			}

			return next(inv)
		}
	}
}

func (r *RootCmd) HeaderTransport(ctx context.Context, serverURL *url.URL) (*codersdk.HeaderTransport, error) {
	transport := &codersdk.HeaderTransport{
		Transport: http.DefaultTransport,
		Header:    http.Header{},
	}
	headers := r.header
	if r.headerCommand != "" {
		shell := "sh"
		caller := "-c"
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
			caller = "/c"
		}
		var outBuf bytes.Buffer
		// #nosec
		cmd := exec.CommandContext(ctx, shell, caller, r.headerCommand)
		cmd.Env = append(os.Environ(), "CODER_URL="+serverURL.String())
		cmd.Stdout = &outBuf
		cmd.Stderr = io.Discard
		err := cmd.Run()
		if err != nil {
			return nil, xerrors.Errorf("failed to run %v: %w", cmd.Args, err)
		}
		scanner := bufio.NewScanner(&outBuf)
		for scanner.Scan() {
			headers = append(headers, scanner.Text())
		}
		if err := scanner.Err(); err != nil {
			return nil, xerrors.Errorf("scan %v: %w", cmd.Args, err)
		}
	}
	for _, header := range headers {
		parts := strings.SplitN(header, "=", 2)
		if len(parts) < 2 {
			return nil, xerrors.Errorf("split header %q had less than two parts", header)
		}
		transport.Header.Add(parts[0], parts[1])
	}
	return transport, nil
}

func (r *RootCmd) setClient(ctx context.Context, client *codersdk.Client, serverURL *url.URL) error {
	transport, err := r.HeaderTransport(ctx, serverURL)
	if err != nil {
		return xerrors.Errorf("create header transport: %w", err)
	}

	client.URL = serverURL
	client.HTTPClient = &http.Client{
		Transport: transport,
	}
	return nil
}

func (r *RootCmd) createUnauthenticatedClient(ctx context.Context, serverURL *url.URL) (*codersdk.Client, error) {
	var client codersdk.Client
	err := r.setClient(ctx, &client, serverURL)
	return &client, err
}

// createAgentClient returns a new client from the command context.
// It works just like CreateClient, but uses the agent token and URL instead.
func (r *RootCmd) createAgentClient() (*agentsdk.Client, error) {
	client := agentsdk.New(r.agentURL)
	client.SetSessionToken(r.agentToken)
	return client, nil
}

// CurrentOrganization returns the currently active organization for the authenticated user.
func CurrentOrganization(inv *clibase.Invocation, client *codersdk.Client) (codersdk.Organization, error) {
	orgs, err := client.OrganizationsByUser(inv.Context(), codersdk.Me)
	if err != nil {
		return codersdk.Organization{}, nil
	}
	// For now, we won't use the config to set this.
	// Eventually, we will support changing using "coder switch <org>"
	return orgs[0], nil
}

func splitNamedWorkspace(identifier string) (owner string, workspaceName string, err error) {
	parts := strings.Split(identifier, "/")

	switch len(parts) {
	case 1:
		owner = codersdk.Me
		workspaceName = parts[0]
	case 2:
		owner = parts[0]
		workspaceName = parts[1]
	default:
		return "", "", xerrors.Errorf("invalid workspace name: %q", identifier)
	}
	return owner, workspaceName, nil
}

// namedWorkspace fetches and returns a workspace by an identifier, which may be either
// a bare name (for a workspace owned by the current user) or a "user/workspace" combination,
// where user is either a username or UUID.
func namedWorkspace(ctx context.Context, client *codersdk.Client, identifier string) (codersdk.Workspace, error) {
	owner, name, err := splitNamedWorkspace(identifier)
	if err != nil {
		return codersdk.Workspace{}, err
	}
	return client.WorkspaceByOwnerAndName(ctx, owner, name, codersdk.WorkspaceOptions{})
}

// createConfig consumes the global configuration flag to produce a config root.
func (r *RootCmd) createConfig() config.Root {
	return config.Root(r.globalConfig)
}

// isTTY returns whether the passed reader is a TTY or not.
func isTTY(inv *clibase.Invocation) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := inv.ParsedFlags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := inv.Stdin.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
}

// isTTYOut returns whether the passed reader is a TTY or not.
func isTTYOut(inv *clibase.Invocation) bool {
	return isTTYWriter(inv, inv.Stdout)
}

// isTTYErr returns whether the passed reader is a TTY or not.
func isTTYErr(inv *clibase.Invocation) bool {
	return isTTYWriter(inv, inv.Stderr)
}

func isTTYWriter(inv *clibase.Invocation, writer io.Writer) bool {
	// If the `--force-tty` command is available, and set,
	// assume we're in a tty. This is primarily for cases on Windows
	// where we may not be able to reliably detect this automatically (ie, tests)
	forceTty, err := inv.ParsedFlags().GetBool(varForceTty)
	if forceTty && err == nil {
		return true
	}
	file, ok := writer.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd())
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
	var sb strings.Builder

	padStyle := cliui.DefaultStyles.Wrap.With(pretty.XPad(4, 0))
	for i, e := range examples {
		if len(e.Description) > 0 {
			wordwrap.WrapString(e.Description, 80)
			_, _ = sb.WriteString(
				"  - " + pretty.Sprint(padStyle, e.Description+":")[4:] + "\n\n    ",
			)
		}
		// We add 1 space here because `cliui.DefaultStyles.Code` adds an extra
		// space. This makes the code block align at an even 2 or 6
		// spaces for symmetry.
		_, _ = sb.WriteString(" " + pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("$ %s", e.Command)))
		if i < len(examples)-1 {
			_, _ = sb.WriteString("\n\n")
		}
	}
	return sb.String()
}

func (r *RootCmd) checkVersions(i *clibase.Invocation, client *codersdk.Client) error {
	if r.noVersionCheck {
		return nil
	}

	ctx, cancel := context.WithTimeout(i.Context(), 10*time.Second)
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
		warn := cliui.DefaultStyles.Warn
		_, _ = fmt.Fprintf(i.Stderr, pretty.Sprint(warn, fmtWarningText), clientVersion, info.Version, strings.TrimPrefix(info.CanonicalVersion(), "v"))
		_, _ = fmt.Fprintln(i.Stderr)
	}

	return nil
}

func (r *RootCmd) checkWarnings(i *clibase.Invocation, client *codersdk.Client) error {
	if r.noFeatureWarning {
		return nil
	}

	ctx, cancel := context.WithTimeout(i.Context(), 10*time.Second)
	defer cancel()

	user, err := client.User(ctx, codersdk.Me)
	if err != nil {
		return xerrors.Errorf("get user me: %w", err)
	}

	entitlements, err := client.Entitlements(ctx)
	if err == nil {
		// Don't show warning to regular users.
		if len(user.Roles) > 0 {
			for _, w := range entitlements.Warnings {
				_, _ = fmt.Fprintln(i.Stderr, pretty.Sprint(cliui.DefaultStyles.Warn, w))
			}
		}
	}
	return nil
}

// Verbosef logs a message if verbose mode is enabled.
func (r *RootCmd) Verbosef(inv *clibase.Invocation, fmtStr string, args ...interface{}) {
	if r.verbose {
		cliui.Infof(inv.Stdout, fmtStr, args...)
	}
}

// DumpHandler provides a custom SIGQUIT and SIGTRAP handler that dumps the
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
func DumpHandler(ctx context.Context) {
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

type exitError struct {
	code int
	err  error
}

var _ error = (*exitError)(nil)

func (e *exitError) Error() string {
	if e.err != nil {
		return fmt.Sprintf("exit code %d: %v", e.code, e.err)
	}
	return fmt.Sprintf("exit code %d", e.code)
}

func (e *exitError) Unwrap() error {
	return e.err
}

// ExitError returns an error that will cause the CLI to exit with the given
// exit code. If err is non-nil, it will be wrapped by the returned error.
func ExitError(code int, err error) error {
	return &exitError{code: code, err: err}
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

type prettyErrorFormatter struct {
	w io.Writer
	// verbose turns on more detailed error logs, such as stack traces.
	verbose bool
}

// format formats the error to the console. This error should be human
// readable.
func (p *prettyErrorFormatter) format(err error) {
	output, _ := cliHumanFormatError("", err, &formatOpts{
		Verbose: p.verbose,
	})
	// always trail with a newline
	_, _ = p.w.Write([]byte(output + "\n"))
}

type formatOpts struct {
	Verbose bool
}

const indent = "    "

// cliHumanFormatError formats an error for the CLI. Newlines and styling are
// included. The second return value is true if the error is special and the error
// chain has custom formatting applied.
//
// If you change this code, you can use the cli "example-errors" tool to
// verify all errors still look ok.
//
//	go run main.go exp example-error <type>
//	       go run main.go exp example-error api
//	       go run main.go exp example-error cmd
//	       go run main.go exp example-error multi-error
//	       go run main.go exp example-error validation
//
//nolint:errorlint
func cliHumanFormatError(from string, err error, opts *formatOpts) (string, bool) {
	if opts == nil {
		opts = &formatOpts{}
	}
	if err == nil {
		return "<nil>", true
	}

	if multi, ok := err.(interface{ Unwrap() []error }); ok {
		multiErrors := multi.Unwrap()
		if len(multiErrors) == 1 {
			// Format as a single error
			return cliHumanFormatError(from, multiErrors[0], opts)
		}
		return formatMultiError(from, multiErrors, opts), true
	}

	// First check for sentinel errors that we want to handle specially.
	// Order does matter! We want to check for the most specific errors first.
	if sdkError, ok := err.(*codersdk.Error); ok {
		return formatCoderSDKError(from, sdkError, opts), true
	}

	if cmdErr, ok := err.(*clibase.RunCommandError); ok {
		// no need to pass the "from" context to this since it is always
		// top level. We care about what is below this.
		return formatRunCommandError(cmdErr, opts), true
	}

	uw, ok := err.(interface{ Unwrap() error })
	if ok {
		msg, special := cliHumanFormatError(from+traceError(err), uw.Unwrap(), opts)
		if special {
			return msg, special
		}
	}
	// If we got here, that means that the wrapped error chain does not have
	// any special formatting below it. So we want to return the topmost non-special
	// error (which is 'err')

	// Default just printing the error. Use +v for verbose to handle stack
	// traces of xerrors.
	if opts.Verbose {
		return pretty.Sprint(headLineStyle(), fmt.Sprintf("%+v", err)), false
	}

	return pretty.Sprint(headLineStyle(), fmt.Sprintf("%v", err)), false
}

// formatMultiError formats a multi-error. It formats it as a list of errors.
//
//	Multiple Errors:
//	<# errors encountered>:
//		1. <heading error message>
//		   <verbose error message>
//		2. <heading error message>
//		   <verbose error message>
func formatMultiError(from string, multi []error, opts *formatOpts) string {
	var errorStrings []string
	for _, err := range multi {
		msg, _ := cliHumanFormatError("", err, opts)
		errorStrings = append(errorStrings, msg)
	}

	// Write errors out
	var str strings.Builder
	var traceMsg string
	if from != "" {
		traceMsg = fmt.Sprintf("Trace=[%s])", from)
	}
	_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("%d errors encountered: %s", len(multi), traceMsg)))
	for i, errStr := range errorStrings {
		// Indent each error
		errStr = strings.ReplaceAll(errStr, "\n", "\n"+indent)
		// Error now looks like
		// |  <line>
		// |  <line>
		prefix := fmt.Sprintf("%d. ", i+1)
		if len(prefix) < len(indent) {
			// Indent the prefix to match the indent
			prefix = prefix + strings.Repeat(" ", len(indent)-len(prefix))
		}
		errStr = prefix + errStr
		// Now looks like
		// |1.<line>
		// |  <line>
		_, _ = str.WriteString("\n" + errStr)
	}
	return str.String()
}

// formatRunCommandError are cli command errors. This kind of error is very
// broad, as it contains all errors that occur when running a command.
// If you know the error is something else, like a codersdk.Error, make a new
// formatter and add it to cliHumanFormatError function.
func formatRunCommandError(err *clibase.RunCommandError, opts *formatOpts) string {
	var str strings.Builder
	_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("Encountered an error running %q", err.Cmd.FullName())))

	msgString, special := cliHumanFormatError("", err.Err, opts)
	_, _ = str.WriteString("\n")
	if special {
		_, _ = str.WriteString(msgString)
	} else {
		_, _ = str.WriteString(pretty.Sprint(tailLineStyle(), msgString))
	}

	return str.String()
}

// formatCoderSDKError come from API requests. In verbose mode, add the
// request debug information.
func formatCoderSDKError(from string, err *codersdk.Error, opts *formatOpts) string {
	var str strings.Builder
	if opts.Verbose {
		_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("API request error to \"%s:%s\". Status code %d", err.Method(), err.URL(), err.StatusCode())))
		_, _ = str.WriteString("\n")
	}
	// Always include this trace. Users can ignore this.
	if from != "" {
		_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("Trace=[%s]", from)))
		_, _ = str.WriteString("\n")
	}

	_, _ = str.WriteString(pretty.Sprint(headLineStyle(), err.Message))
	if err.Helper != "" {
		_, _ = str.WriteString("\n")
		_, _ = str.WriteString(pretty.Sprint(tailLineStyle(), err.Helper))
	}
	// By default we do not show the Detail with the helper.
	if opts.Verbose || (err.Helper == "" && err.Detail != "") {
		_, _ = str.WriteString("\n")
		_, _ = str.WriteString(pretty.Sprint(tailLineStyle(), err.Detail))
	}
	return str.String()
}

// traceError is a helper function that aides developers debugging failed cli
// commands. When we pretty print errors, we lose the context in which they came.
// This function adds the context back. Unfortunately there is no easy way to get
// the prefix to: "error string: %w", so we do a bit of string manipulation.
//
//nolint:errorlint
func traceError(err error) string {
	if uw, ok := err.(interface{ Unwrap() error }); ok {
		a, b := err.Error(), uw.Unwrap().Error()
		c := strings.TrimSuffix(a, b)
		return c
	}
	return err.Error()
}

// These styles are arbitrary.
func headLineStyle() pretty.Style {
	return cliui.DefaultStyles.Error
}

func tailLineStyle() pretty.Style {
	return pretty.Style{pretty.Nop}
}

//nolint:unused
func SlimUnsupported(w io.Writer, cmd string) {
	_, _ = fmt.Fprintf(w, "You are using a 'slim' build of Coder, which does not support the %s subcommand.\n", pretty.Sprint(cliui.DefaultStyles.Code, cmd))
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Please use a build of Coder from GitHub releases:")
	_, _ = fmt.Fprintln(w, "  https://github.com/coder/coder/releases")

	//nolint:revive
	os.Exit(1)
}
