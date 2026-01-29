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
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/trace"
	"slices"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/google/uuid"
	"github.com/mattn/go-isatty"
	"github.com/mitchellh/go-wordwrap"
	"golang.org/x/mod/semver"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/cli/gitauth"
	"github.com/coder/coder/v2/cli/sessionstore"
	"github.com/coder/coder/v2/cli/telemetry"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

var (
	Caret = pretty.Sprint(cliui.DefaultStyles.Prompt, "")

	// Applied as annotations to workspace commands
	// so they display in a separated "help" section.
	workspaceCommand = map[string]string{
		"workspaces": "",
	}

	// ErrSilent is a sentinel error that tells the command handler to just exit with a non-zero error, but not print
	// anything.
	ErrSilent = xerrors.New("silent error")

	errKeyringNotSupported = xerrors.New("keyring storage is not supported on this operating system; omit --use-keyring to use file-based storage")
)

const (
	varURL                     = "url"
	varToken                   = "token"
	varHeader                  = "header"
	varHeaderCommand           = "header-command"
	varNoOpen                  = "no-open"
	varNoVersionCheck          = "no-version-warning"
	varNoFeatureWarning        = "no-feature-warning"
	varForceTty                = "force-tty"
	varVerbose                 = "verbose"
	varDisableDirect           = "disable-direct-connections"
	varDisableNetworkTelemetry = "disable-network-telemetry"
	varUseKeyring              = "use-keyring"

	notLoggedInMessage = "You are not logged in. Try logging in using '%s login <url>'."

	envNoVersionCheck   = "CODER_NO_VERSION_WARNING"
	envNoFeatureWarning = "CODER_NO_FEATURE_WARNING"
	envSessionToken     = "CODER_SESSION_TOKEN"
	envUseKeyring       = "CODER_USE_KEYRING"
	//nolint:gosec
	envAgentToken = "CODER_AGENT_TOKEN"
	//nolint:gosec
	envAgentTokenFile = "CODER_AGENT_TOKEN_FILE"
	envAgentURL       = "CODER_AGENT_URL"
	envAgentAuth      = "CODER_AGENT_AUTH"
	envURL            = "CODER_URL"
)

func (r *RootCmd) CoreSubcommands() []*serpent.Command {
	// Please re-sort this list alphabetically if you change it!
	return []*serpent.Command{
		r.completion(),
		r.dotfiles(),
		externalAuth(),
		r.login(),
		r.logout(),
		r.netcheck(),
		r.notifications(),
		r.organizations(),
		r.portForward(),
		r.publickey(),
		r.resetPassword(),
		r.sharing(),
		r.state(),
		r.tasksCommand(),
		r.templates(),
		r.tokens(),
		r.users(),
		r.version(defaultVersionInfo),

		// Workspace Commands
		r.autoupdate(),
		r.configSSH(),
		r.Create(CreateOptions{}),
		r.deleteWorkspace(),
		r.favorite(),
		r.list(),
		r.logs(),
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
		r.unfavorite(),
		r.update(),
		r.whoami(),

		// Hidden
		r.connectCmd(),
		gitssh(),
		r.support(),
		r.vpnDaemon(),
		r.vscodeSSH(),
		workspaceAgent(),
	}
}

// AGPLExperimental returns all AGPL experimental subcommands.
func (r *RootCmd) AGPLExperimental() []*serpent.Command {
	return []*serpent.Command{
		r.scaletestCmd(),
		r.errorExample(),
		r.mcpCommand(),
		r.promptExample(),
		r.rptyCommand(),
		r.syncCommand(),
	}
}

// AGPL returns all AGPL commands including any non-core commands that are
// duplicated in the Enterprise CLI.
func (r *RootCmd) AGPL() []*serpent.Command {
	all := append(
		r.CoreSubcommands(),
		r.Server( /* Do not import coderd here. */ nil),
		r.Provisioners(),
		ExperimentalCommand(r.AGPLExperimental()),
	)
	return all
}

// ExperimentalCommand creates an experimental command that is hidden and has
// the given subcommands.
func ExperimentalCommand(subcommands []*serpent.Command) *serpent.Command {
	cmd := &serpent.Command{
		Use:   "exp",
		Short: "Internal commands for testing and experimentation. These are prone to breaking changes with no notice.",
		Handler: func(i *serpent.Invocation) error {
			return i.Command.HelpHandler(i)
		},
		Hidden:   true,
		Children: subcommands,
	}
	return cmd
}

// RunWithSubcommands runs the root command with the given subcommands.
// It is abstracted to enable the Enterprise code to add commands.
func (r *RootCmd) RunWithSubcommands(subcommands []*serpent.Command) {
	// This configuration is not available as a standard option because we
	// want to trace the entire program, including Options parsing.
	goTraceFilePath, ok := os.LookupEnv("CODER_GO_TRACE")
	if ok {
		traceFile, err := os.OpenFile(goTraceFilePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			panic(fmt.Sprintf("failed to open trace file: %v", err))
		}
		defer traceFile.Close()

		if err := trace.Start(traceFile); err != nil {
			panic(fmt.Sprintf("failed to start trace: %v", err))
		}
		defer trace.Stop()
	}

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
		if errors.Is(err, cliui.ErrCanceled) {
			//nolint:revive,gocritic
			os.Exit(code)
		}
		if errors.Is(err, ErrSilent) {
			//nolint:revive,gocritic
			os.Exit(code)
		}
		f := PrettyErrorFormatter{w: os.Stderr, verbose: r.verbose}
		if err != nil {
			f.Format(err)
		}
		//nolint:revive,gocritic
		os.Exit(code)
	}
}

func (r *RootCmd) Command(subcommands []*serpent.Command) (*serpent.Command, error) {
	fmtLong := `Coder %s — A tool for provisioning self-hosted development environments with Terraform.
`
	hiddenAgentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use: "coder [global-flags] <subcommand>",
		Long: fmt.Sprintf(fmtLong, buildinfo.Version()) + FormatExamples(
			Example{
				Description: "Start a Coder server",
				Command:     "coder server",
			},
			Example{
				Description: "Get started by creating a template from an example",
				Command:     "coder templates init",
			},
		),
		Handler: func(i *serpent.Invocation) error {
			if r.versionFlag {
				return r.version(defaultVersionInfo).Handler(i)
			}
			// The GIT_ASKPASS environment variable must point at
			// a binary with no arguments. To prevent writing
			// cross-platform scripts to invoke the Coder binary
			// with a `gitaskpass` subcommand, we override the entrypoint
			// to check if the command was invoked.
			if gitauth.CheckCommand(i.Args, i.Environ.ToOS()) {
				return gitAskpass(hiddenAgentAuth).Handler(i)
			}
			return i.Command.HelpHandler(i)
		},
	}

	cmd.AddSubcommands(subcommands...)

	// Set default help handler for all commands.
	cmd.Walk(func(c *serpent.Command) {
		if c.HelpHandler == nil {
			c.HelpHandler = helpFn()
		}
	})

	var merr error
	// Add [flags] to usage when appropriate.
	cmd.Walk(func(cmd *serpent.Command) {
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

	// Add aliases when appropriate.
	cmd.Walk(func(cmd *serpent.Command) {
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
	cmd.Walk(func(cmd *serpent.Command) {
		for _, opt := range cmd.Options {
			// Verify that every option is configurable.
			if opt.Flag == "" && opt.Env == "" {
				if cmd.Name() == "server" {
					// The server command is funky and has YAML-only options, e.g.
					// support links.
					return
				}
				if cmd.Name() == "boundary" {
					// The boundary command is integrated from the boundary package
					// and has YAML-only options (e.g., allowlist from config file)
					// that don't have flags or env vars.
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
	cmd.Walk(func(cmd *serpent.Command) {
		h := cmd.Handler
		if h == nil {
			// We should never have a nil handler, but if we do, do not
			// wrap it. Wrapping it just hides a nil pointer dereference.
			// If a nil handler exists, this is a developer bug. If no handler
			// is required for a command such as command grouping (e.g. `users'
			// and 'groups'), then the handler should be set to the helper
			// function.
			//	func(inv *serpent.Invocation) error {
			//		return inv.Command.HelpHandler(inv)
			//	}
			return
		}
		cmd.Handler = func(i *serpent.Invocation) error {
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

	// Add the PrintDeprecatedOptions middleware to all commands.
	cmd.Walk(func(cmd *serpent.Command) {
		if cmd.Middleware == nil {
			cmd.Middleware = PrintDeprecatedOptions()
		} else {
			cmd.Middleware = serpent.Chain(cmd.Middleware, PrintDeprecatedOptions())
		}
	})

	if r.clientURL == nil {
		r.clientURL = new(url.URL)
	}

	globalGroup := &serpent.Group{
		Name:        "Global",
		Description: `Global options are applied to all commands. They can be set using environment variables or flags.`,
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:        varURL,
			Env:         envURL,
			Description: "URL to a deployment.",
			Value:       serpent.URLOf(r.clientURL),
			Group:       globalGroup,
		},
		{
			Flag:        "debug-options",
			Description: "Print all options, how they're set, then exit.",
			Value:       serpent.BoolOf(&debugOptions),
			Group:       globalGroup,
		},
		{
			Flag:        varToken,
			Env:         envSessionToken,
			Description: fmt.Sprintf("Specify an authentication token. For security reasons setting %s is preferred.", envSessionToken),
			Value:       serpent.StringOf(&r.token),
			Group:       globalGroup,
		},
		{
			Flag:        varNoVersionCheck,
			Env:         envNoVersionCheck,
			Description: "Suppress warning when client and server versions do not match.",
			Value:       serpent.BoolOf(&r.noVersionCheck),
			Group:       globalGroup,
		},
		{
			Flag:        varNoFeatureWarning,
			Env:         envNoFeatureWarning,
			Description: "Suppress warnings about unlicensed features.",
			Value:       serpent.BoolOf(&r.noFeatureWarning),
			Group:       globalGroup,
		},
		{
			Flag:        varHeader,
			Env:         "CODER_HEADER",
			Description: "Additional HTTP headers added to all requests. Provide as " + `key=value` + ". Can be specified multiple times.",
			Value:       serpent.StringArrayOf(&r.header),
			Group:       globalGroup,
		},
		{
			Flag:        varHeaderCommand,
			Env:         "CODER_HEADER_COMMAND",
			Description: "An external command that outputs additional HTTP headers added to all requests. The command must output each header as `key=value` on its own line.",
			Value:       serpent.StringOf(&r.headerCommand),
			Group:       globalGroup,
		},
		{
			Flag:        varNoOpen,
			Env:         "CODER_NO_OPEN",
			Description: "Suppress opening the browser when logging in, or starting the server.",
			Value:       serpent.BoolOf(&r.noOpen),
			Hidden:      true,
			Group:       globalGroup,
		},
		{
			Flag:        varForceTty,
			Env:         "CODER_FORCE_TTY",
			Hidden:      false,
			Description: "Force the use of a TTY.",
			Value:       serpent.BoolOf(&r.forceTTY),
			Group:       globalGroup,
		},
		{
			Flag:          varVerbose,
			FlagShorthand: "v",
			Env:           "CODER_VERBOSE",
			Description:   "Enable verbose output.",
			Value:         serpent.BoolOf(&r.verbose),
			Group:         globalGroup,
		},
		{
			Flag:        varDisableDirect,
			Env:         "CODER_DISABLE_DIRECT_CONNECTIONS",
			Description: "Disable direct (P2P) connections to workspaces.",
			Value:       serpent.BoolOf(&r.disableDirect),
			Group:       globalGroup,
		},
		{
			Flag:        varDisableNetworkTelemetry,
			Env:         "CODER_DISABLE_NETWORK_TELEMETRY",
			Description: "Disable network telemetry. Network telemetry is collected when connecting to workspaces using the CLI, and is forwarded to the server. If telemetry is also enabled on the server, it may be sent to Coder. Network telemetry is used to measure network quality and detect regressions.",
			Value:       serpent.BoolOf(&r.disableNetworkTelemetry),
			Group:       globalGroup,
		},
		{
			Flag: varUseKeyring,
			Env:  envUseKeyring,
			Description: "Store and retrieve session tokens using the operating system " +
				"keyring. This flag is ignored and file-based storage is used when " +
				"--global-config is set or keyring usage is not supported on the current " +
				"platform. Set to false to force file-based storage on supported platforms.",
			Default: "true",
			Value:   serpent.BoolOf(&r.useKeyring),
			Group:   globalGroup,
		},
		{
			Flag:        "debug-http",
			Description: "Debug codersdk HTTP requests.",
			Value:       serpent.BoolOf(&r.debugHTTP),
			Group:       globalGroup,
			Hidden:      true,
		},
		{
			Flag:        config.FlagName,
			Env:         "CODER_CONFIG_DIR",
			Description: "Path to the global `coder` config directory.",
			Default:     config.DefaultDir(),
			Value:       serpent.StringOf(&r.globalConfig),
			Group:       globalGroup,
		},
		{
			Flag: "version",
			// This was requested by a customer to assist with their migration.
			// They have two Coder CLIs, and want to tell the difference by running
			// the same base command.
			Description: "Run the version command. Useful for v1 customers migrating to v2.",
			Value:       serpent.BoolOf(&r.versionFlag),
			Hidden:      true,
		},
	}
	hiddenAgentAuth.AttachOptions(cmd, true)

	return cmd, nil
}

// RootCmd contains parameters and helpers useful to all commands.
type RootCmd struct {
	clientURL     *url.URL
	token         string
	tokenBackend  sessionstore.Backend
	globalConfig  string
	header        []string
	headerCommand string

	forceTTY      bool
	noOpen        bool
	verbose       bool
	versionFlag   bool
	disableDirect bool
	debugHTTP     bool

	disableNetworkTelemetry    bool
	noVersionCheck             bool
	noFeatureWarning           bool
	useKeyring                 bool
	keyringServiceName         string
	useKeyringWithGlobalConfig bool
}

// InitClient creates and configures a new client with authentication, telemetry,
// and version checks.
func (r *RootCmd) InitClient(inv *serpent.Invocation) (*codersdk.Client, error) {
	conf := r.createConfig()
	var err error
	// Read the client URL stored on disk.
	if r.clientURL == nil || r.clientURL.String() == "" {
		rawURL, err := conf.URL().Read()
		// If the configuration files are absent, the user is logged out
		if os.IsNotExist(err) {
			binPath, err := os.Executable()
			if err != nil {
				binPath = "coder"
			}
			return nil, xerrors.Errorf(notLoggedInMessage, binPath)
		}
		if err != nil {
			return nil, err
		}

		r.clientURL, err = url.Parse(strings.TrimSpace(rawURL))
		if err != nil {
			return nil, err
		}
	}
	if r.token == "" {
		tok, err := r.ensureTokenBackend().Read(r.clientURL)
		// Even if there isn't a token, we don't care.
		// Some API routes can be unauthenticated.
		if err != nil && !xerrors.Is(err, os.ErrNotExist) {
			if xerrors.Is(err, sessionstore.ErrNotImplemented) {
				return nil, errKeyringNotSupported
			}
			return nil, err
		}
		if tok != "" {
			r.token = tok
		}
	}

	// Configure HTTP client with transport wrappers
	httpClient, err := r.createHTTPClient(inv.Context(), r.clientURL, inv)
	if err != nil {
		return nil, err
	}

	clientOpts := []codersdk.ClientOption{
		codersdk.WithSessionToken(r.token),
		codersdk.WithHTTPClient(httpClient),
	}

	if r.disableDirect {
		clientOpts = append(clientOpts, codersdk.WithDisableDirectConnections())
	}

	if r.debugHTTP {
		clientOpts = append(clientOpts,
			codersdk.WithPlainLogger(os.Stderr),
			codersdk.WithLogBodies(),
		)
	}

	return codersdk.New(r.clientURL, clientOpts...), nil
}

// TryInitClient is similar to InitClient but doesn't error when credentials are missing.
// This allows commands to run without requiring authentication, but still use auth if available.
func (r *RootCmd) TryInitClient(inv *serpent.Invocation) (*codersdk.Client, error) {
	conf := r.createConfig()
	// Read the client URL stored on disk.
	if r.clientURL == nil || r.clientURL.String() == "" {
		rawURL, err := conf.URL().Read()
		// If the configuration files are absent, just continue without URL
		if err != nil {
			// Continue with a nil or empty URL
			if !os.IsNotExist(err) {
				return nil, err
			}
		} else {
			r.clientURL, err = url.Parse(strings.TrimSpace(rawURL))
			if err != nil {
				return nil, err
			}
		}
	}
	if r.token == "" {
		tok, err := r.ensureTokenBackend().Read(r.clientURL)
		// Even if there isn't a token, we don't care.
		// Some API routes can be unauthenticated.
		if err != nil && !xerrors.Is(err, os.ErrNotExist) {
			if xerrors.Is(err, sessionstore.ErrNotImplemented) {
				return nil, errKeyringNotSupported
			}
			return nil, err
		}
		if tok != "" {
			r.token = tok
		}
	}

	// Only configure the client if we have a URL
	if r.clientURL != nil && r.clientURL.String() != "" {
		// Configure HTTP client with transport wrappers
		httpClient, err := r.createHTTPClient(inv.Context(), r.clientURL, inv)
		if err != nil {
			return nil, err
		}

		clientOpts := []codersdk.ClientOption{
			codersdk.WithSessionToken(r.token),
			codersdk.WithHTTPClient(httpClient),
		}

		if r.disableDirect {
			clientOpts = append(clientOpts, codersdk.WithDisableDirectConnections())
		}

		if r.debugHTTP {
			clientOpts = append(clientOpts,
				codersdk.WithPlainLogger(os.Stderr),
				codersdk.WithLogBodies(),
			)
		}

		return codersdk.New(r.clientURL, clientOpts...), nil
	}

	// Return a minimal client if no URL is available
	return &codersdk.Client{}, nil
}

// HeaderTransport creates a new transport that executes `--header-command`
// if it is set to add headers for all outbound requests.
func (r *RootCmd) HeaderTransport(ctx context.Context, serverURL *url.URL) (*codersdk.HeaderTransport, error) {
	return headerTransport(ctx, serverURL, r.header, r.headerCommand)
}

func (r *RootCmd) createHTTPClient(ctx context.Context, serverURL *url.URL, inv *serpent.Invocation) (*http.Client, error) {
	transport := http.DefaultTransport
	transport = wrapTransportWithTelemetryHeader(transport, inv)
	transport = wrapTransportWithUserAgentHeader(transport, inv)
	if !r.noVersionCheck {
		transport = wrapTransportWithVersionMismatchCheck(transport, inv, buildinfo.Version(), func(ctx context.Context) (codersdk.BuildInfoResponse, error) {
			// Create a new client without any wrapped transport
			// otherwise it creates an infinite loop!
			basicClient := codersdk.New(serverURL)
			return basicClient.BuildInfo(ctx)
		})
	}
	if !r.noFeatureWarning {
		transport = wrapTransportWithEntitlementsCheck(transport, inv.Stderr)
	}
	headerTransport, err := r.HeaderTransport(ctx, serverURL)
	if err != nil {
		return nil, xerrors.Errorf("create header transport: %w", err)
	}
	// The header transport has to come last.
	// codersdk checks for the header transport to get headers
	// to clone on the DERP client.
	headerTransport.Transport = transport
	return &http.Client{
		Transport: headerTransport,
	}, nil
}

func (r *RootCmd) createUnauthenticatedClient(ctx context.Context, serverURL *url.URL, inv *serpent.Invocation) (*codersdk.Client, error) {
	httpClient, err := r.createHTTPClient(ctx, serverURL, inv)
	if err != nil {
		return nil, err
	}
	client := codersdk.New(serverURL, codersdk.WithHTTPClient(httpClient))
	return client, nil
}

// ensureTokenBackend returns the session token storage backend, creating it if necessary.
// This must be called after flags are parsed so we can respect the value of the --use-keyring
// flag.
func (r *RootCmd) ensureTokenBackend() sessionstore.Backend {
	if r.tokenBackend == nil {
		// Checking for the --global-config directory being set is a bit wonky but necessary
		// to allow extensions that invoke the CLI with this flag (e.g. VS code) to continue
		// working without modification. In the future we should modify these extensions to
		// either access the credential in the keyring (like Coder Desktop) or some other
		// approach that doesn't rely on the session token being stored on disk.
		assumeExtensionInUse := r.globalConfig != config.DefaultDir() && !r.useKeyringWithGlobalConfig
		keyringSupported := runtime.GOOS == "windows" || runtime.GOOS == "darwin"
		if r.useKeyring && !assumeExtensionInUse && keyringSupported {
			serviceName := sessionstore.DefaultServiceName
			if r.keyringServiceName != "" {
				serviceName = r.keyringServiceName
			}
			r.tokenBackend = sessionstore.NewKeyringWithService(serviceName)
		} else {
			r.tokenBackend = sessionstore.NewFile(r.createConfig)
		}
	}
	return r.tokenBackend
}

// WithKeyringServiceName sets a custom keyring service name for testing purposes.
// This allows tests to use isolated keyring storage while still exercising the
// genuine storage backend selection logic in ensureTokenBackend().
func (r *RootCmd) WithKeyringServiceName(serviceName string) {
	r.keyringServiceName = serviceName
}

// UseKeyringWithGlobalConfig enables the use of the keyring storage backend
// when the --global-config directory is set. This is only intended as an override
// for tests, which require specifying the global config directory for test isolation.
func (r *RootCmd) UseKeyringWithGlobalConfig() {
	r.useKeyringWithGlobalConfig = true
}

type AgentAuth struct {
	// Agent Client config
	agentToken     string
	agentTokenFile string
	agentURL       url.URL
	agentAuth      string
}

func (a *AgentAuth) AttachOptions(cmd *serpent.Command, hidden bool) {
	cmd.Options = append(cmd.Options, serpent.Option{
		Name:        "Agent Token",
		Description: "An agent authentication token.",
		Flag:        "agent-token",
		Env:         envAgentToken,
		Value:       serpent.StringOf(&a.agentToken),
		Hidden:      hidden,
	}, serpent.Option{
		Name:        "Agent Token File",
		Description: "A file containing an agent authentication token.",
		Flag:        "agent-token-file",
		Env:         envAgentTokenFile,
		Value:       serpent.StringOf(&a.agentTokenFile),
		Hidden:      hidden,
	}, serpent.Option{
		Name:        "Agent URL",
		Description: "URL for an agent to access your deployment.",
		Flag:        "agent-url",
		Env:         envAgentURL,
		Value:       serpent.URLOf(&a.agentURL),
		Hidden:      hidden,
	}, serpent.Option{
		Name:        "Agent Auth",
		Description: "Specify the authentication type to use for the agent.",
		Flag:        "auth",
		Env:         envAgentAuth,
		Default:     "token",
		Value:       serpent.StringOf(&a.agentAuth),
		Hidden:      hidden,
	})
}

// CreateClient returns a new agent client from the command context.  It works
// just like InitClient, but uses the agent token and URL instead.
func (a *AgentAuth) CreateClient() (*agentsdk.Client, error) {
	agentURL := a.agentURL
	if agentURL.String() == "" {
		return nil, xerrors.Errorf("%s must be set", envAgentURL)
	}

	switch a.agentAuth {
	case "token":
		token := a.agentToken
		if token == "" {
			if a.agentTokenFile == "" {
				return nil, xerrors.Errorf("Either %s or %s must be set", envAgentToken, envAgentTokenFile)
			}
			tokenBytes, err := os.ReadFile(a.agentTokenFile)
			if err != nil {
				return nil, xerrors.Errorf("read token file %q: %w", a.agentTokenFile, err)
			}
			token = strings.TrimSpace(string(tokenBytes))
		}
		if token == "" {
			return nil, xerrors.Errorf("CODER_AGENT_TOKEN or CODER_AGENT_TOKEN_FILE must be set for token auth")
		}
		return agentsdk.New(&a.agentURL, agentsdk.WithFixedToken(token)), nil
	case "google-instance-identity":
		return agentsdk.New(&a.agentURL, agentsdk.WithGoogleInstanceIdentity("", nil)), nil
	case "aws-instance-identity":
		return agentsdk.New(&a.agentURL, agentsdk.WithAWSInstanceIdentity()), nil
	case "azure-instance-identity":
		return agentsdk.New(&a.agentURL, agentsdk.WithAzureInstanceIdentity()), nil
	default:
		return nil, xerrors.Errorf("unknown agent auth type: %s", a.agentAuth)
	}
}

type OrganizationContext struct {
	// FlagSelect is the value passed in via the --org flag
	FlagSelect string
}

func NewOrganizationContext() *OrganizationContext {
	return &OrganizationContext{}
}

func (*OrganizationContext) optionName() string { return "Organization" }
func (o *OrganizationContext) AttachOptions(cmd *serpent.Command) {
	cmd.Options = append(cmd.Options, serpent.Option{
		Name:        o.optionName(),
		Description: "Select which organization (uuid or name) to use.",
		// Only required if the user is a part of more than 1 organization.
		// Otherwise, we can assume a default value.
		Required:      false,
		Flag:          "org",
		FlagShorthand: "O",
		Env:           "CODER_ORGANIZATION",
		Value:         serpent.StringOf(&o.FlagSelect),
	})
}

func (o *OrganizationContext) ValueSource(inv *serpent.Invocation) (string, serpent.ValueSource) {
	opt := inv.Command.Options.ByName(o.optionName())
	if opt == nil {
		return o.FlagSelect, serpent.ValueSourceNone
	}
	return o.FlagSelect, opt.ValueSource
}

func (o *OrganizationContext) Selected(inv *serpent.Invocation, client *codersdk.Client) (codersdk.Organization, error) {
	// Fetch the set of organizations the user is a member of.
	orgs, err := client.OrganizationsByUser(inv.Context(), codersdk.Me)
	if err != nil {
		return codersdk.Organization{}, xerrors.Errorf("get organizations: %w", err)
	}

	// User manually selected an organization
	if o.FlagSelect != "" {
		index := slices.IndexFunc(orgs, func(org codersdk.Organization) bool {
			return org.Name == o.FlagSelect || org.ID.String() == o.FlagSelect
		})

		if index < 0 {
			var names []string
			for _, org := range orgs {
				names = append(names, org.Name)
			}
			return codersdk.Organization{}, xerrors.Errorf("organization %q not found, are you sure you are a member of this organization? "+
				"Valid options for '--org=' are [%s].", o.FlagSelect, strings.Join(names, ", "))
		}
		return orgs[index], nil
	}

	if len(orgs) == 1 {
		return orgs[0], nil
	}

	// No org selected, and we are more than 1? Return an error.
	validOrgs := make([]string, 0, len(orgs))
	for _, org := range orgs {
		validOrgs = append(validOrgs, org.Name)
	}

	return codersdk.Organization{}, xerrors.Errorf("Must select an organization with --org=<org_name>. Choose from: %s", strings.Join(validOrgs, ", "))
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
	if uid, err := uuid.Parse(identifier); err == nil {
		return client.Workspace(ctx, uid)
	}
	owner, name, err := splitNamedWorkspace(identifier)
	if err != nil {
		return codersdk.Workspace{}, err
	}
	return client.WorkspaceByOwnerAndName(ctx, owner, name, codersdk.WorkspaceOptions{})
}

func initAppearance(ctx context.Context, client *codersdk.Client) codersdk.AppearanceConfig {
	// best effort
	cfg, _ := client.Appearance(ctx)
	if cfg.DocsURL == "" {
		cfg.DocsURL = codersdk.DefaultDocsURL()
	}
	return cfg
}

// createConfig consumes the global configuration flag to produce a config root.
func (r *RootCmd) createConfig() config.Root {
	return config.Root(r.globalConfig)
}

// isTTYIn returns whether the passed invocation is having stdin read from a TTY
func isTTYIn(inv *serpent.Invocation) bool {
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

// isTTYOut returns whether the passed invocation is having stdout written to a TTY
func isTTYOut(inv *serpent.Invocation) bool {
	return isTTYWriter(inv, inv.Stdout)
}

// isTTYErr returns whether the passed invocation is having stderr written to a TTY
func isTTYErr(inv *serpent.Invocation) bool {
	return isTTYWriter(inv, inv.Stderr)
}

func isTTYWriter(inv *serpent.Invocation, writer io.Writer) bool {
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

// Example represents a standard example for command usage, to be used
// with FormatExamples.
type Example struct {
	Description string
	Command     string
}

// FormatExamples formats the examples as width wrapped bulletpoint
// descriptions with the command underneath.
func FormatExamples(examples ...Example) string {
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

// Verbosef logs a message if verbose mode is enabled.
func (r *RootCmd) Verbosef(inv *serpent.Invocation, fmtStr string, args ...interface{}) {
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
func DumpHandler(ctx context.Context, name string) {
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
		// Make the time filesystem-safe, for example ":" is not
		// permitted on many filesystems. Note that Z here only appends
		// Z to the string, it does not actually change the time zone.
		filesystemSafeTime := time.Now().UTC().Format("2006-01-02T15-04-05.000Z")
		fpath := filepath.Join(dir, fmt.Sprintf("coder-%s-%s.dump", name, filesystemSafeTime))
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
			//nolint:revive,gocritic
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

// NewPrettyErrorFormatter creates a new PrettyErrorFormatter.
func NewPrettyErrorFormatter(w io.Writer, verbose bool) *PrettyErrorFormatter {
	return &PrettyErrorFormatter{
		w:       w,
		verbose: verbose,
	}
}

type PrettyErrorFormatter struct {
	w io.Writer
	// verbose turns on more detailed error logs, such as stack traces.
	verbose bool
}

// Format formats the error to the writer in PrettyErrorFormatter.
// This error should be human readable.
func (p *PrettyErrorFormatter) Format(err error) {
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

	if cmdErr, ok := err.(*serpent.RunCommandError); ok {
		// no need to pass the "from" context to this since it is always
		// top level. We care about what is below this.
		return formatRunCommandError(cmdErr, opts), true
	}

	if uw, ok := err.(interface{ Unwrap() error }); ok {
		if unwrapped := uw.Unwrap(); unwrapped != nil {
			msg, special := cliHumanFormatError(from+traceError(err), unwrapped, opts)
			if special {
				return msg, special
			}
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
			prefix += strings.Repeat(" ", len(indent)-len(prefix))
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
func formatRunCommandError(err *serpent.RunCommandError, opts *formatOpts) string {
	var str strings.Builder
	_, _ = str.WriteString(pretty.Sprint(headLineStyle(),
		fmt.Sprintf(
			`Encountered an error running %q, see "%s --help" for more information`,
			err.Cmd.FullName(), err.Cmd.FullName())))
	_, _ = str.WriteString(pretty.Sprint(headLineStyle(), "\nerror: "))

	msgString, special := cliHumanFormatError("", err.Err, opts)
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
		// If all these fields are empty, then do not print this information.
		// This can occur if the error is being used outside the api.
		if err.Method() != "" || err.URL() != "" || err.StatusCode() != 0 {
			_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("API request error to \"%s:%s\". Status code %d", err.Method(), err.URL(), err.StatusCode())))
			_, _ = str.WriteString("\n")
		}
	}
	// Always include this trace. Users can ignore this.
	if from != "" {
		_, _ = str.WriteString(pretty.Sprint(headLineStyle(), fmt.Sprintf("Trace=[%s]", from)))
		_, _ = str.WriteString("\n")
	}

	// The main error message
	_, _ = str.WriteString(pretty.Sprint(headLineStyle(), err.Message))

	// Validation errors.
	if len(err.Validations) > 0 {
		_, _ = str.WriteString("\n")
		_, _ = str.WriteString(pretty.Sprint(tailLineStyle(), fmt.Sprintf("%d validation error(s) found", len(err.Validations))))
		for _, e := range err.Validations {
			_, _ = str.WriteString("\n\t")
			_, _ = str.WriteString(pretty.Sprint(cliui.DefaultStyles.Field, e.Field))
			_, _ = str.WriteString(pretty.Sprintf(cliui.DefaultStyles.Warn, ": %s", e.Detail))
		}
	}

	if err.Helper != "" {
		_, _ = str.WriteString("\n")
		_, _ = str.WriteString(pretty.Sprintf(tailLineStyle(), "Suggestion: %s", err.Helper))
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
		var a, b string
		if err != nil {
			a = err.Error()
		}
		if uw != nil {
			uwerr := uw.Unwrap()
			if uwerr != nil {
				b = uwerr.Error()
			}
		}
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

func defaultUpgradeMessage(version string) string {
	// Our installation script doesn't work on Windows, so instead we direct the user
	// to the GitHub release page to download the latest installer.
	version = strings.TrimPrefix(version, "v")
	if runtime.GOOS == "windows" {
		return fmt.Sprintf("download the server version from: https://github.com/coder/coder/releases/v%s", version)
	}
	return fmt.Sprintf("download the server version with: 'curl -L https://coder.com/install.sh | sh -s -- --version %s'", version)
}

// wrapTransportWithEntitlementsCheck adds a middleware to the HTTP transport
// that checks for entitlement warnings and prints them to the user.
func wrapTransportWithEntitlementsCheck(rt http.RoundTripper, w io.Writer) http.RoundTripper {
	var once sync.Once
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		res, err := rt.RoundTrip(req)
		if err != nil {
			return res, err
		}
		once.Do(func() {
			for _, warning := range res.Header.Values(codersdk.EntitlementsWarningHeader) {
				_, _ = fmt.Fprintln(w, pretty.Sprint(cliui.DefaultStyles.Warn, warning))
			}
		})
		return res, err
	})
}

// wrapTransportWithVersionMismatchCheck adds a middleware to the HTTP transport
// that checks for version mismatches between the client and server. If a mismatch
// is detected, a warning is printed to the user.
func wrapTransportWithVersionMismatchCheck(rt http.RoundTripper, inv *serpent.Invocation, clientVersion string, getBuildInfo func(ctx context.Context) (codersdk.BuildInfoResponse, error)) http.RoundTripper {
	var once sync.Once
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		res, err := rt.RoundTrip(req)
		if err != nil {
			return res, err
		}
		once.Do(func() {
			serverVersion := res.Header.Get(codersdk.BuildVersionHeader)
			if serverVersion == "" {
				return
			}
			if buildinfo.VersionsMatch(clientVersion, serverVersion) {
				return
			}
			upgradeMessage := defaultUpgradeMessage(semver.Canonical(serverVersion))
			if serverInfo, err := getBuildInfo(inv.Context()); err == nil {
				switch {
				case serverInfo.UpgradeMessage != "":
					upgradeMessage = serverInfo.UpgradeMessage
				// The site-local `install.sh` was introduced in v2.19.0
				case serverInfo.DashboardURL != "" && semver.Compare(semver.MajorMinor(serverVersion), "v2.19") >= 0:
					upgradeMessage = fmt.Sprintf("download %s with: 'curl -fsSL %s/install.sh | sh'", serverVersion, serverInfo.DashboardURL)
				}
			}
			fmtWarningText := "version mismatch: client %s, server %s\n%s"
			fmtWarn := pretty.Sprint(cliui.DefaultStyles.Warn, fmtWarningText)
			warning := fmt.Sprintf(fmtWarn, clientVersion, serverVersion, upgradeMessage)

			_, _ = fmt.Fprintln(inv.Stderr, warning)
		})
		return res, err
	})
}

// wrapTransportWithTelemetryHeader adds telemetry headers to report command usage
// to an HTTP transport.
func wrapTransportWithTelemetryHeader(transport http.RoundTripper, inv *serpent.Invocation) http.RoundTripper {
	var (
		value string
		once  sync.Once
	)
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		once.Do(func() {
			// We only want to compute this header once when a request
			// first goes out, hence the complexity with locking here.
			var topts []telemetry.Option
			for _, opt := range inv.Command.FullOptions() {
				if opt.ValueSource == serpent.ValueSourceNone || opt.ValueSource == serpent.ValueSourceDefault {
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
			s := base64.StdEncoding.EncodeToString(byt)
			// Don't send the header if it's too long!
			if len(s) <= 4096 {
				value = s
			}
		})
		if value != "" {
			req.Header.Add(codersdk.CLITelemetryHeader, value)
		}
		return transport.RoundTrip(req)
	})
}

// wrapTransportWithUserAgentHeader sets a User-Agent header for all CLI requests
// that includes the CLI version, os/arch, and the specific command being run.
func wrapTransportWithUserAgentHeader(transport http.RoundTripper, inv *serpent.Invocation) http.RoundTripper {
	var (
		userAgent string
		once      sync.Once
	)
	return roundTripper(func(req *http.Request) (*http.Response, error) {
		once.Do(func() {
			userAgent = fmt.Sprintf("coder-cli/%s (%s/%s; %s)", buildinfo.Version(), runtime.GOOS, runtime.GOARCH, inv.Command.FullName())
		})
		req.Header.Set("User-Agent", userAgent)
		return transport.RoundTrip(req)
	})
}

type roundTripper func(req *http.Request) (*http.Response, error)

func (r roundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return r(req)
}

// HeaderTransport creates a new transport that executes `--header-command`
// if it is set to add headers for all outbound requests.
func headerTransport(ctx context.Context, serverURL *url.URL, header []string, headerCommand string) (*codersdk.HeaderTransport, error) {
	transport := &codersdk.HeaderTransport{
		Transport: http.DefaultTransport,
		Header:    http.Header{},
	}
	headers := header
	if headerCommand != "" {
		shell := "sh"
		caller := "-c"
		if runtime.GOOS == "windows" {
			shell = "cmd.exe"
			caller = "/c"
		}
		var outBuf bytes.Buffer
		// #nosec
		cmd := exec.CommandContext(ctx, shell, caller, headerCommand)
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

// printDeprecatedOptions loops through all command options, and prints
// a warning for usage of deprecated options.
func PrintDeprecatedOptions() serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(inv *serpent.Invocation) error {
			opts := inv.Command.Options
			// Print deprecation warnings.
			for _, opt := range opts {
				if opt.UseInstead == nil {
					continue
				}

				if opt.ValueSource == serpent.ValueSourceNone || opt.ValueSource == serpent.ValueSourceDefault {
					continue
				}

				var warnStr strings.Builder
				_, _ = warnStr.WriteString(translateSource(opt.ValueSource, opt))
				_, _ = warnStr.WriteString(" is deprecated, please use ")
				for i, use := range opt.UseInstead {
					_, _ = warnStr.WriteString(translateSource(opt.ValueSource, use))
					if i != len(opt.UseInstead)-1 {
						_, _ = warnStr.WriteString(" and ")
					}
				}
				_, _ = warnStr.WriteString(" instead.\n")

				cliui.Warn(inv.Stderr,
					warnStr.String(),
				)
			}

			return next(inv)
		}
	}
}

// translateSource provides the name of the source of the option, depending on the
// supplied target ValueSource.
func translateSource(target serpent.ValueSource, opt serpent.Option) string {
	switch target {
	case serpent.ValueSourceFlag:
		return fmt.Sprintf("`--%s`", opt.Flag)
	case serpent.ValueSourceEnv:
		return fmt.Sprintf("`%s`", opt.Env)
	case serpent.ValueSourceYAML:
		return fmt.Sprintf("`%s`", fullYamlName(opt))
	default:
		return opt.Name
	}
}

func fullYamlName(opt serpent.Option) string {
	var full strings.Builder
	for _, name := range opt.Group.Ancestry() {
		_, _ = full.WriteString(name.YAML)
		_, _ = full.WriteString(".")
	}
	_, _ = full.WriteString(opt.YAML)
	return full.String()
}
