package cli

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/xerrors"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"cdr.dev/slog/sloggers/slogjson"
	"cdr.dev/slog/sloggers/slogstackdriver"
	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentproc"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/reaper"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) workspaceAgent() *serpent.Command {
	var (
		auth                string
		logDir              string
		scriptDataDir       string
		pprofAddress        string
		noReap              bool
		sshMaxTimeout       time.Duration
		tailnetListenPort   int64
		prometheusAddress   string
		debugAddress        string
		slogHumanPath       string
		slogJSONPath        string
		slogStackdriverPath string
		blockFileTransfer   bool
		agentHeaderCommand  string
		agentHeader         []string
	)
	cmd := &serpent.Command{
		Use:   "agent",
		Short: `Starts the Coder workspace agent.`,
		// This command isn't useful to manually execute.
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			var (
				ignorePorts = map[int]string{}
				isLinux     = runtime.GOOS == "linux"

				sinks      = []slog.Sink{}
				logClosers = []func() error{}
			)
			defer func() {
				for _, closer := range logClosers {
					_ = closer()
				}
			}()

			addSinkIfProvided := func(sinkFn func(io.Writer) slog.Sink, loc string) error {
				switch loc {
				case "":
					// Do nothing.

				case "/dev/stderr":
					sinks = append(sinks, sinkFn(inv.Stderr))

				case "/dev/stdout":
					sinks = append(sinks, sinkFn(inv.Stdout))

				default:
					fi, err := os.OpenFile(loc, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
					if err != nil {
						return xerrors.Errorf("open log file %q: %w", loc, err)
					}
					sinks = append(sinks, sinkFn(fi))
					logClosers = append(logClosers, fi.Close)
				}
				return nil
			}

			if err := addSinkIfProvided(sloghuman.Sink, slogHumanPath); err != nil {
				return xerrors.Errorf("add human sink: %w", err)
			}
			if err := addSinkIfProvided(slogjson.Sink, slogJSONPath); err != nil {
				return xerrors.Errorf("add json sink: %w", err)
			}
			if err := addSinkIfProvided(slogstackdriver.Sink, slogStackdriverPath); err != nil {
				return xerrors.Errorf("add stackdriver sink: %w", err)
			}

			// Spawn a reaper so that we don't accumulate a ton
			// of zombie processes.
			if reaper.IsInitProcess() && !noReap && isLinux {
				logWriter := &lumberjackWriteCloseFixer{w: &lumberjack.Logger{
					Filename: filepath.Join(logDir, "coder-agent-init.log"),
					MaxSize:  5, // MB
					// Without this, rotated logs will never be deleted.
					MaxBackups: 1,
				}}
				defer logWriter.Close()

				sinks = append(sinks, sloghuman.Sink(logWriter))
				logger := inv.Logger.AppendSinks(sinks...).Leveled(slog.LevelDebug)

				logger.Info(ctx, "spawning reaper process")
				// Do not start a reaper on the child process. It's important
				// to do this else we fork bomb ourselves.
				args := append(os.Args, "--no-reap")
				err := reaper.ForkReap(
					reaper.WithExecArgs(args...),
					reaper.WithCatchSignals(StopSignals...),
				)
				if err != nil {
					logger.Error(ctx, "agent process reaper unable to fork", slog.Error(err))
					return xerrors.Errorf("fork reap: %w", err)
				}

				logger.Info(ctx, "reaper process exiting")
				return nil
			}

			// Handle interrupt signals to allow for graceful shutdown,
			// note that calling stopNotify disables the signal handler
			// and the next interrupt will terminate the program (you
			// probably want cancel instead).
			//
			// Note that we don't want to handle these signals in the
			// process that runs as PID 1, that's why we do this after
			// the reaper forked.
			ctx, stopNotify := inv.SignalNotifyContext(ctx, StopSignals...)
			defer stopNotify()

			// DumpHandler does signal handling, so we call it after the
			// reaper.
			go DumpHandler(ctx, "agent")

			logWriter := &lumberjackWriteCloseFixer{w: &lumberjack.Logger{
				Filename: filepath.Join(logDir, "coder-agent.log"),
				MaxSize:  5, // MB
				// Per customer incident on November 17th, 2023, its helpful
				// to have the log of the last few restarts to debug a failing agent.
				MaxBackups: 10,
			}}
			defer logWriter.Close()

			sinks = append(sinks, sloghuman.Sink(logWriter))
			logger := inv.Logger.AppendSinks(sinks...).Leveled(slog.LevelDebug)

			version := buildinfo.Version()
			logger.Info(ctx, "agent is starting now",
				slog.F("url", r.agentURL),
				slog.F("auth", auth),
				slog.F("version", version),
			)
			client := agentsdk.New(r.agentURL)
			client.SDK.SetLogger(logger)
			// Set a reasonable timeout so requests can't hang forever!
			// The timeout needs to be reasonably long, because requests
			// with large payloads can take a bit. e.g. startup scripts
			// may take a while to insert.
			client.SDK.HTTPClient.Timeout = 30 * time.Second
			// Attach header transport so we process --agent-header and
			// --agent-header-command flags
			headerTransport, err := headerTransport(ctx, r.agentURL, agentHeader, agentHeaderCommand)
			if err != nil {
				return xerrors.Errorf("configure header transport: %w", err)
			}
			headerTransport.Transport = client.SDK.HTTPClient.Transport
			client.SDK.HTTPClient.Transport = headerTransport

			// Enable pprof handler
			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			pprofSrvClose := ServeHandler(ctx, logger, nil, pprofAddress, "pprof")
			defer pprofSrvClose()
			if port, err := extractPort(pprofAddress); err == nil {
				ignorePorts[port] = "pprof"
			}

			if port, err := extractPort(prometheusAddress); err == nil {
				ignorePorts[port] = "prometheus"
			}

			if port, err := extractPort(debugAddress); err == nil {
				ignorePorts[port] = "debug"
			}

			// exchangeToken returns a session token.
			// This is abstracted to allow for the same looping condition
			// regardless of instance identity auth type.
			var exchangeToken func(context.Context) (agentsdk.AuthenticateResponse, error)
			switch auth {
			case "token":
				token, _ := inv.ParsedFlags().GetString(varAgentToken)
				if token == "" {
					tokenFile, _ := inv.ParsedFlags().GetString(varAgentTokenFile)
					if tokenFile != "" {
						tokenBytes, err := os.ReadFile(tokenFile)
						if err != nil {
							return xerrors.Errorf("read token file %q: %w", tokenFile, err)
						}
						token = strings.TrimSpace(string(tokenBytes))
					}
				}
				if token == "" {
					return xerrors.Errorf("CODER_AGENT_TOKEN or CODER_AGENT_TOKEN_FILE must be set for token auth")
				}
				client.SetSessionToken(token)
			case "google-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var gcpClient *metadata.Client
				gcpClientRaw := ctx.Value("gcp-client")
				if gcpClientRaw != nil {
					gcpClient, _ = gcpClientRaw.(*metadata.Client)
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthGoogleInstanceIdentity(ctx, "", gcpClient)
				}
			case "aws-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var awsClient *http.Client
				awsClientRaw := ctx.Value("aws-client")
				if awsClientRaw != nil {
					awsClient, _ = awsClientRaw.(*http.Client)
					if awsClient != nil {
						client.SDK.HTTPClient = awsClient
					}
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthAWSInstanceIdentity(ctx)
				}
			case "azure-instance-identity":
				// This is *only* done for testing to mock client authentication.
				// This will never be set in a production scenario.
				var azureClient *http.Client
				azureClientRaw := ctx.Value("azure-client")
				if azureClientRaw != nil {
					azureClient, _ = azureClientRaw.(*http.Client)
					if azureClient != nil {
						client.SDK.HTTPClient = azureClient
					}
				}
				exchangeToken = func(ctx context.Context) (agentsdk.AuthenticateResponse, error) {
					return client.AuthAzureInstanceIdentity(ctx)
				}
			}

			executablePath, err := os.Executable()
			if err != nil {
				return xerrors.Errorf("getting os executable: %w", err)
			}
			err = os.Setenv("PATH", fmt.Sprintf("%s%c%s", os.Getenv("PATH"), filepath.ListSeparator, filepath.Dir(executablePath)))
			if err != nil {
				return xerrors.Errorf("add executable to $PATH: %w", err)
			}

			prometheusRegistry := prometheus.NewRegistry()
			subsystemsRaw := inv.Environ.Get(agent.EnvAgentSubsystem)
			subsystems := []codersdk.AgentSubsystem{}
			for _, s := range strings.Split(subsystemsRaw, ",") {
				subsystem := codersdk.AgentSubsystem(strings.TrimSpace(s))
				if subsystem == "" {
					continue
				}
				if !subsystem.Valid() {
					return xerrors.Errorf("invalid subsystem %q", subsystem)
				}
				subsystems = append(subsystems, subsystem)
			}

			environmentVariables := map[string]string{
				"GIT_ASKPASS": executablePath,
			}
			if v, ok := os.LookupEnv(agent.EnvProcPrioMgmt); ok {
				environmentVariables[agent.EnvProcPrioMgmt] = v
			}
			if v, ok := os.LookupEnv(agent.EnvProcOOMScore); ok {
				environmentVariables[agent.EnvProcOOMScore] = v
			}

			agnt := agent.New(agent.Options{
				Client:            client,
				Logger:            logger,
				LogDir:            logDir,
				ScriptDataDir:     scriptDataDir,
				TailnetListenPort: uint16(tailnetListenPort),
				ExchangeToken: func(ctx context.Context) (string, error) {
					if exchangeToken == nil {
						return client.SDK.SessionToken(), nil
					}
					resp, err := exchangeToken(ctx)
					if err != nil {
						return "", err
					}
					client.SetSessionToken(resp.SessionToken)
					return resp.SessionToken, nil
				},
				EnvironmentVariables: environmentVariables,
				IgnorePorts:          ignorePorts,
				SSHMaxTimeout:        sshMaxTimeout,
				Subsystems:           subsystems,

				PrometheusRegistry: prometheusRegistry,
				Syscaller:          agentproc.NewSyscaller(),
				// Intentionally set this to nil. It's mainly used
				// for testing.
				ModifiedProcesses: nil,

				BlockFileTransfer: blockFileTransfer,
			})

			promHandler := agent.PrometheusMetricsHandler(prometheusRegistry, logger)
			prometheusSrvClose := ServeHandler(ctx, logger, promHandler, prometheusAddress, "prometheus")
			defer prometheusSrvClose()

			debugSrvClose := ServeHandler(ctx, logger, agnt.HTTPDebug(), debugAddress, "debug")
			defer debugSrvClose()

			<-ctx.Done()
			return agnt.Close()
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Flag:        "auth",
			Default:     "token",
			Description: "Specify the authentication type to use for the agent.",
			Env:         "CODER_AGENT_AUTH",
			Value:       serpent.StringOf(&auth),
		},
		{
			Flag:        "log-dir",
			Default:     os.TempDir(),
			Description: "Specify the location for the agent log files.",
			Env:         "CODER_AGENT_LOG_DIR",
			Value:       serpent.StringOf(&logDir),
		},
		{
			Flag:        "script-data-dir",
			Default:     os.TempDir(),
			Description: "Specify the location for storing script data.",
			Env:         "CODER_AGENT_SCRIPT_DATA_DIR",
			Value:       serpent.StringOf(&scriptDataDir),
		},
		{
			Flag:        "pprof-address",
			Default:     "127.0.0.1:6060",
			Env:         "CODER_AGENT_PPROF_ADDRESS",
			Value:       serpent.StringOf(&pprofAddress),
			Description: "The address to serve pprof.",
		},
		{
			Flag:        "agent-header-command",
			Env:         "CODER_AGENT_HEADER_COMMAND",
			Value:       serpent.StringOf(&agentHeaderCommand),
			Description: "An external command that outputs additional HTTP headers added to all requests. The command must output each header as `key=value` on its own line.",
		},
		{
			Flag:        "agent-header",
			Env:         "CODER_AGENT_HEADER",
			Value:       serpent.StringArrayOf(&agentHeader),
			Description: "Additional HTTP headers added to all requests. Provide as " + `key=value` + ". Can be specified multiple times.",
		},
		{
			Flag: "no-reap",

			Env:         "",
			Description: "Do not start a process reaper.",
			Value:       serpent.BoolOf(&noReap),
		},
		{
			Flag: "ssh-max-timeout",
			// tcpip.KeepaliveIdleOption = 72h + 1min (forwardTCPSockOpts() in tailnet/conn.go)
			Default:     "72h",
			Env:         "CODER_AGENT_SSH_MAX_TIMEOUT",
			Description: "Specify the max timeout for a SSH connection, it is advisable to set it to a minimum of 60s, but no more than 72h.",
			Value:       serpent.DurationOf(&sshMaxTimeout),
		},
		{
			Flag:        "tailnet-listen-port",
			Default:     "0",
			Env:         "CODER_AGENT_TAILNET_LISTEN_PORT",
			Description: "Specify a static port for Tailscale to use for listening.",
			Value:       serpent.Int64Of(&tailnetListenPort),
		},
		{
			Flag:        "prometheus-address",
			Default:     "127.0.0.1:2112",
			Env:         "CODER_AGENT_PROMETHEUS_ADDRESS",
			Value:       serpent.StringOf(&prometheusAddress),
			Description: "The bind address to serve Prometheus metrics.",
		},
		{
			Flag:        "debug-address",
			Default:     "127.0.0.1:2113",
			Env:         "CODER_AGENT_DEBUG_ADDRESS",
			Value:       serpent.StringOf(&debugAddress),
			Description: "The bind address to serve a debug HTTP server.",
		},
		{
			Name:        "Human Log Location",
			Description: "Output human-readable logs to a given file.",
			Flag:        "log-human",
			Env:         "CODER_AGENT_LOGGING_HUMAN",
			Default:     "/dev/stderr",
			Value:       serpent.StringOf(&slogHumanPath),
		},
		{
			Name:        "JSON Log Location",
			Description: "Output JSON logs to a given file.",
			Flag:        "log-json",
			Env:         "CODER_AGENT_LOGGING_JSON",
			Default:     "",
			Value:       serpent.StringOf(&slogJSONPath),
		},
		{
			Name:        "Stackdriver Log Location",
			Description: "Output Stackdriver compatible logs to a given file.",
			Flag:        "log-stackdriver",
			Env:         "CODER_AGENT_LOGGING_STACKDRIVER",
			Default:     "",
			Value:       serpent.StringOf(&slogStackdriverPath),
		},
		{
			Flag:        "block-file-transfer",
			Default:     "false",
			Env:         "CODER_AGENT_BLOCK_FILE_TRANSFER",
			Description: fmt.Sprintf("Block file transfer using known applications: %s.", strings.Join(agentssh.BlockedFileTransferCommands, ",")),
			Value:       serpent.BoolOf(&blockFileTransfer),
		},
	}

	return cmd
}

func ServeHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	logger.Debug(ctx, "http server listening", slog.F("addr", addr), slog.F("name", name))

	// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
	// websockets over the dev tunnel.
	// See: https://github.com/coder/coder/pull/3730
	//nolint:gosec
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		err := srv.ListenAndServe()
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.Error(err))
		}
	}()

	return func() {
		_ = srv.Close()
	}
}

// lumberjackWriteCloseFixer is a wrapper around an io.WriteCloser that
// prevents writes after Close. This is necessary because lumberjack
// re-opens the file on Write.
type lumberjackWriteCloseFixer struct {
	w      io.WriteCloser
	mu     sync.Mutex // Protects following.
	closed bool
}

func (c *lumberjackWriteCloseFixer) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	return c.w.Close()
}

func (c *lumberjackWriteCloseFixer) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return 0, io.ErrClosedPipe
	}
	return c.w.Write(p)
}

// extractPort handles different url strings.
// - localhost:6060
// - http://localhost:6060
func extractPort(u string) (int, error) {
	port, firstError := urlPort(u)
	if firstError == nil {
		return port, nil
	}

	// Try with a scheme
	port, err := urlPort("http://" + u)
	if err == nil {
		return port, nil
	}
	return -1, xerrors.Errorf("invalid url %q: %w", u, firstError)
}

// urlPort extracts the port from a valid URL.
func urlPort(u string) (int, error) {
	parsed, err := url.Parse(u)
	if err != nil {
		return -1, xerrors.Errorf("invalid url %q: %w", u, err)
	}
	if parsed.Port() != "" {
		port, err := strconv.ParseUint(parsed.Port(), 10, 16)
		if err == nil && port > 0 && port < 1<<16 {
			return int(port), nil
		}
	}
	return -1, xerrors.Errorf("invalid port: %s", u)
}
