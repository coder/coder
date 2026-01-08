package cli

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"
	"gopkg.in/natefinch/lumberjack.v2"

	"github.com/prometheus/client_golang/prometheus"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"cdr.dev/slog/v3/sloggers/slogjson"
	"cdr.dev/slog/v3/sloggers/slogstackdriver"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/agent"
	"github.com/coder/coder/v2/agent/agentcontainers"
	"github.com/coder/coder/v2/agent/agentexec"
	"github.com/coder/coder/v2/agent/agentssh"
	"github.com/coder/coder/v2/agent/boundarylogproxy"
	"github.com/coder/coder/v2/agent/reaper"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/agentsdk"
)

func workspaceAgent() *serpent.Command {
	var (
		logDir                         string
		scriptDataDir                  string
		pprofAddress                   string
		noReap                         bool
		sshMaxTimeout                  time.Duration
		tailnetListenPort              int64
		prometheusAddress              string
		debugAddress                   string
		slogHumanPath                  string
		slogJSONPath                   string
		slogStackdriverPath            string
		blockFileTransfer              bool
		agentHeaderCommand             string
		agentHeader                    []string
		devcontainers                  bool
		devcontainerProjectDiscovery   bool
		devcontainerDiscoveryAutostart bool
		socketServerEnabled            bool
		socketPath                     string
		boundaryLogProxySocketPath     string
	)
	agentAuth := &AgentAuth{}
	cmd := &serpent.Command{
		Use:   "agent",
		Short: `Starts the Coder workspace agent.`,
		// This command isn't useful to manually execute.
		Hidden: true,
		Handler: func(inv *serpent.Invocation) error {
			ctx, cancel := context.WithCancelCause(inv.Context())
			defer func() {
				cancel(xerrors.New("agent exited"))
			}()

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
				logWriter := &clilog.LumberjackWriteCloseFixer{Writer: &lumberjack.Logger{
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
				//nolint:gocritic
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

			logWriter := &clilog.LumberjackWriteCloseFixer{Writer: &lumberjack.Logger{
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
				slog.F("url", agentAuth.agentURL),
				slog.F("auth", agentAuth.agentAuth),
				slog.F("version", version),
			)
			client, err := agentAuth.CreateClient()
			if err != nil {
				return xerrors.Errorf("create agent client: %w", err)
			}
			client.SDK.SetLogger(logger)
			// Set a reasonable timeout so requests can't hang forever!
			// The timeout needs to be reasonably long, because requests
			// with large payloads can take a bit. e.g. startup scripts
			// may take a while to insert.
			client.SDK.HTTPClient.Timeout = 30 * time.Second
			// Attach header transport so we process --agent-header and
			// --agent-header-command flags
			headerTransport, err := headerTransport(ctx, &agentAuth.agentURL, agentHeader, agentHeaderCommand)
			if err != nil {
				return xerrors.Errorf("configure header transport: %w", err)
			}
			headerTransport.Transport = client.SDK.HTTPClient.Transport
			client.SDK.HTTPClient.Transport = headerTransport

			// Enable pprof handler
			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			if pprofAddress != "" {
				pprofSrvClose := ServeHandler(ctx, logger, nil, pprofAddress, "pprof")
				defer pprofSrvClose()

				if port, err := extractPort(pprofAddress); err == nil {
					ignorePorts[port] = "pprof"
				}
			} else {
				logger.Debug(ctx, "pprof address is empty, disabling pprof server")
			}

			executablePath, err := os.Executable()
			if err != nil {
				return xerrors.Errorf("getting os executable: %w", err)
			}
			err = os.Setenv("PATH", fmt.Sprintf("%s%c%s", os.Getenv("PATH"), filepath.ListSeparator, filepath.Dir(executablePath)))
			if err != nil {
				return xerrors.Errorf("add executable to $PATH: %w", err)
			}

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

			enabled := os.Getenv(agentexec.EnvProcPrioMgmt)
			if enabled != "" && runtime.GOOS == "linux" {
				logger.Info(ctx, "process priority management enabled",
					slog.F("env_var", agentexec.EnvProcPrioMgmt),
					slog.F("enabled", enabled),
					slog.F("os", runtime.GOOS),
				)
			} else {
				logger.Info(ctx, "process priority management not enabled (linux-only) ",
					slog.F("env_var", agentexec.EnvProcPrioMgmt),
					slog.F("enabled", enabled),
					slog.F("os", runtime.GOOS),
				)
			}

			execer, err := agentexec.NewExecer()
			if err != nil {
				return xerrors.Errorf("create agent execer: %w", err)
			}

			if devcontainers {
				logger.Info(ctx, "agent devcontainer detection enabled")
			} else {
				logger.Info(ctx, "agent devcontainer detection not enabled")
			}

			reinitEvents := agentsdk.WaitForReinitLoop(ctx, logger, client)

			var (
				lastErr  error
				mustExit bool
			)
			for {
				prometheusRegistry := prometheus.NewRegistry()

				promHandler := agent.PrometheusMetricsHandler(prometheusRegistry, logger)
				var serverClose []func()
				if prometheusAddress != "" {
					prometheusSrvClose := ServeHandler(ctx, logger, promHandler, prometheusAddress, "prometheus")
					serverClose = append(serverClose, prometheusSrvClose)

					if port, err := extractPort(prometheusAddress); err == nil {
						ignorePorts[port] = "prometheus"
					}
				} else {
					logger.Debug(ctx, "prometheus address is empty, disabling prometheus server")
				}

				if debugAddress != "" {
					// ServerHandle depends on `agnt.HTTPDebug()`, but `agnt`
					// depends on `ignorePorts`. Keep this if statement in sync
					// with below.
					if port, err := extractPort(debugAddress); err == nil {
						ignorePorts[port] = "debug"
					}
				}

				agnt := agent.New(agent.Options{
					Client:        client,
					Logger:        logger,
					LogDir:        logDir,
					ScriptDataDir: scriptDataDir,
					// #nosec G115 - Safe conversion as tailnet listen port is within uint16 range (0-65535)
					TailnetListenPort:    uint16(tailnetListenPort),
					EnvironmentVariables: environmentVariables,
					IgnorePorts:          ignorePorts,
					SSHMaxTimeout:        sshMaxTimeout,
					Subsystems:           subsystems,

					PrometheusRegistry: prometheusRegistry,
					BlockFileTransfer:  blockFileTransfer,
					Execer:             execer,
					Devcontainers:      devcontainers,
					DevcontainerAPIOptions: []agentcontainers.Option{
						agentcontainers.WithSubAgentURL(agentAuth.agentURL.String()),
						agentcontainers.WithProjectDiscovery(devcontainerProjectDiscovery),
						agentcontainers.WithDiscoveryAutostart(devcontainerDiscoveryAutostart),
					},
					SocketPath:                 socketPath,
					SocketServerEnabled:        socketServerEnabled,
					BoundaryLogProxySocketPath: boundaryLogProxySocketPath,
				})

				if debugAddress != "" {
					// ServerHandle depends on `agnt.HTTPDebug()`, but `agnt`
					// depends on `ignorePorts`. Keep this if statement in sync
					// with above.
					debugSrvClose := ServeHandler(ctx, logger, agnt.HTTPDebug(), debugAddress, "debug")
					serverClose = append(serverClose, debugSrvClose)
				} else {
					logger.Debug(ctx, "debug address is empty, disabling debug server")
				}

				select {
				case <-ctx.Done():
					logger.Info(ctx, "agent shutting down", slog.Error(context.Cause(ctx)))
					mustExit = true
				case event := <-reinitEvents:
					logger.Info(ctx, "agent received instruction to reinitialize",
						slog.F("workspace_id", event.WorkspaceID), slog.F("reason", event.Reason))
				}

				lastErr = agnt.Close()

				slices.Reverse(serverClose)
				for _, closeFunc := range serverClose {
					closeFunc()
				}

				if mustExit {
					break
				}

				logger.Info(ctx, "agent reinitializing")
			}
			return lastErr
		},
	}

	cmd.Options = serpent.OptionSet{
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
		{
			Flag:        "devcontainers-enable",
			Default:     "true",
			Env:         "CODER_AGENT_DEVCONTAINERS_ENABLE",
			Description: "Allow the agent to automatically detect running devcontainers.",
			Value:       serpent.BoolOf(&devcontainers),
		},
		{
			Flag:        "devcontainers-project-discovery-enable",
			Default:     "true",
			Env:         "CODER_AGENT_DEVCONTAINERS_PROJECT_DISCOVERY_ENABLE",
			Description: "Allow the agent to search the filesystem for devcontainer projects.",
			Value:       serpent.BoolOf(&devcontainerProjectDiscovery),
		},
		{
			Flag:        "devcontainers-discovery-autostart-enable",
			Default:     "false",
			Env:         "CODER_AGENT_DEVCONTAINERS_DISCOVERY_AUTOSTART_ENABLE",
			Description: "Allow the agent to autostart devcontainer projects it discovers based on their configuration.",
			Value:       serpent.BoolOf(&devcontainerDiscoveryAutostart),
		},
		{
			Flag:        "socket-server-enabled",
			Default:     "false",
			Env:         "CODER_AGENT_SOCKET_SERVER_ENABLED",
			Description: "Enable the agent socket server.",
			Value:       serpent.BoolOf(&socketServerEnabled),
		},
		{
			Flag:        "socket-path",
			Env:         "CODER_AGENT_SOCKET_PATH",
			Description: "Specify the path for the agent socket.",
			Value:       serpent.StringOf(&socketPath),
		},
		{
			Flag:        "boundary-log-proxy-socket-path",
			Default:     boundarylogproxy.DefaultSocketPath(),
			Env:         "CODER_AGENT_BOUNDARY_LOG_PROXY_SOCKET_PATH",
			Description: "The path for the boundary log proxy server Unix socket. Boundary should write audit logs to this socket.",
			Value:       serpent.StringOf(&boundaryLogProxySocketPath),
		},
	}
	agentAuth.AttachOptions(cmd, false)
	return cmd
}

func ServeHandler(ctx context.Context, logger slog.Logger, handler http.Handler, addr, name string) (closeFunc func()) {
	// ReadHeaderTimeout is purposefully not enabled. It caused some issues with
	// websockets over the dev tunnel.
	// See: https://github.com/coder/coder/pull/3730
	//nolint:gosec
	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	go func() {
		ln, err := net.Listen("tcp", addr)
		if err != nil {
			logger.Error(ctx, "http server listen", slog.F("name", name), slog.F("addr", addr), slog.Error(err))
			return
		}
		defer ln.Close()
		logger.Info(ctx, "http server listening", slog.F("addr", ln.Addr()), slog.F("name", name))
		if err := srv.Serve(ln); err != nil && !xerrors.Is(err, http.ErrServerClosed) {
			logger.Error(ctx, "http server serve", slog.F("addr", ln.Addr()), slog.F("name", name), slog.Error(err))
		}
	}()

	return func() {
		_ = srv.Close()
	}
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
