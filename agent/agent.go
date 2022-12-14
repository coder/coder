package agent

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"
	"github.com/pkg/sftp"
	"github.com/spf13/afero"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/exp/slices"
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"
	"tailscale.com/types/netlogtype"

	"cdr.dev/slog"
	"github.com/coder/coder/agent/usershell"
	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/coderd/gitauth"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/pty"
	"github.com/coder/coder/tailnet"
	"github.com/coder/retry"
)

const (
	ProtocolReconnectingPTY = "reconnecting-pty"
	ProtocolSSH             = "ssh"
	ProtocolDial            = "dial"

	// MagicSessionErrorCode indicates that something went wrong with the session, rather than the
	// command just returning a nonzero exit code, and is chosen as an arbitrary, high number
	// unlikely to shadow other exit codes, which are typically 1, 2, 3, etc.
	MagicSessionErrorCode = 229
)

type Options struct {
	Filesystem             afero.Fs
	TempDir                string
	ExchangeToken          func(ctx context.Context) (string, error)
	Client                 Client
	ReconnectingPTYTimeout time.Duration
	EnvironmentVariables   map[string]string
	Logger                 slog.Logger
}

type Client interface {
	WorkspaceAgentMetadata(ctx context.Context) (codersdk.WorkspaceAgentMetadata, error)
	ListenWorkspaceAgent(ctx context.Context) (net.Conn, error)
	AgentReportStats(ctx context.Context, log slog.Logger, stats func() *codersdk.AgentStats) (io.Closer, error)
	PostWorkspaceAgentAppHealth(ctx context.Context, req codersdk.PostWorkspaceAppHealthsRequest) error
	PostWorkspaceAgentVersion(ctx context.Context, version string) error
}

func New(options Options) io.Closer {
	if options.ReconnectingPTYTimeout == 0 {
		options.ReconnectingPTYTimeout = 5 * time.Minute
	}
	if options.Filesystem == nil {
		options.Filesystem = afero.NewOsFs()
	}
	if options.TempDir == "" {
		options.TempDir = os.TempDir()
	}
	if options.ExchangeToken == nil {
		options.ExchangeToken = func(ctx context.Context) (string, error) {
			return "", nil
		}
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	a := &agent{
		reconnectingPTYTimeout: options.ReconnectingPTYTimeout,
		logger:                 options.Logger,
		closeCancel:            cancelFunc,
		closed:                 make(chan struct{}),
		envVars:                options.EnvironmentVariables,
		client:                 options.Client,
		exchangeToken:          options.ExchangeToken,
		filesystem:             options.Filesystem,
		tempDir:                options.TempDir,
	}
	a.init(ctx)
	return a
}

type agent struct {
	logger        slog.Logger
	client        Client
	exchangeToken func(ctx context.Context) (string, error)
	filesystem    afero.Fs
	tempDir       string

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	envVars map[string]string
	// metadata is atomic because values can change after reconnection.
	metadata     atomic.Value
	sessionToken atomic.Pointer[string]
	sshServer    *ssh.Server

	network *tailnet.Conn
}

// runLoop attempts to start the agent in a retry loop.
// Coder may be offline temporarily, a connection issue
// may be happening, but regardless after the intermittent
// failure, you'll want the agent to reconnect.
func (a *agent) runLoop(ctx context.Context) {
	for retrier := retry.New(100*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		a.logger.Info(ctx, "running loop")
		err := a.run(ctx)
		// Cancel after the run is complete to clean up any leaked resources!
		if err == nil {
			continue
		}
		if errors.Is(err, context.Canceled) {
			return
		}
		if a.isClosed() {
			return
		}
		if errors.Is(err, io.EOF) {
			a.logger.Info(ctx, "likely disconnected from coder", slog.Error(err))
			continue
		}
		a.logger.Warn(ctx, "run exited with error", slog.Error(err))
	}
}

func (a *agent) run(ctx context.Context) error {
	// This allows the agent to refresh it's token if necessary.
	// For instance identity this is required, since the instance
	// may not have re-provisioned, but a new agent ID was created.
	sessionToken, err := a.exchangeToken(ctx)
	if err != nil {
		return xerrors.Errorf("exchange token: %w", err)
	}
	a.sessionToken.Store(&sessionToken)

	err = a.client.PostWorkspaceAgentVersion(ctx, buildinfo.Version())
	if err != nil {
		return xerrors.Errorf("update workspace agent version: %w", err)
	}

	metadata, err := a.client.WorkspaceAgentMetadata(ctx)
	if err != nil {
		return xerrors.Errorf("fetch metadata: %w", err)
	}
	a.logger.Info(ctx, "fetched metadata")
	oldMetadata := a.metadata.Swap(metadata)

	// The startup script should only execute on the first run!
	if oldMetadata == nil {
		go func() {
			err := a.runStartupScript(ctx, metadata.StartupScript)
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				a.logger.Warn(ctx, "agent script failed", slog.Error(err))
			}
		}()
	}

	if metadata.GitAuthConfigs > 0 {
		err = gitauth.OverrideVSCodeConfigs(a.filesystem)
		if err != nil {
			return xerrors.Errorf("override vscode configuration for git auth: %w", err)
		}
	}

	// This automatically closes when the context ends!
	appReporterCtx, appReporterCtxCancel := context.WithCancel(ctx)
	defer appReporterCtxCancel()
	go NewWorkspaceAppHealthReporter(
		a.logger, metadata.Apps, a.client.PostWorkspaceAgentAppHealth)(appReporterCtx)

	a.logger.Debug(ctx, "running tailnet with derpmap", slog.F("derpmap", metadata.DERPMap))

	a.closeMutex.Lock()
	network := a.network
	a.closeMutex.Unlock()
	if network == nil {
		a.logger.Debug(ctx, "creating tailnet")
		network, err = a.createTailnet(ctx, metadata.DERPMap)
		if err != nil {
			return xerrors.Errorf("create tailnet: %w", err)
		}
		a.closeMutex.Lock()
		// Re-check if agent was closed while initializing the network.
		closed := a.isClosed()
		if !closed {
			a.network = network
		}
		a.closeMutex.Unlock()
		if closed {
			_ = network.Close()
			return xerrors.New("agent is closed")
		}

		// Report statistics from the created network.
		cl, err := a.client.AgentReportStats(ctx, a.logger, func() *codersdk.AgentStats {
			stats := network.ExtractTrafficStats()
			return convertAgentStats(stats)
		})
		if err != nil {
			a.logger.Error(ctx, "report stats", slog.Error(err))
		} else {
			if err = a.trackConnGoroutine(func() {
				// This is OK because the agent never re-creates the tailnet
				// and the only shutdown indicator is agent.Close().
				<-a.closed
				_ = cl.Close()
			}); err != nil {
				a.logger.Debug(ctx, "report stats goroutine", slog.Error(err))
				_ = cl.Close()
			}
		}
	} else {
		// Update the DERP map!
		network.SetDERPMap(metadata.DERPMap)
	}

	a.logger.Debug(ctx, "running coordinator")
	err = a.runCoordinator(ctx, network)
	if err != nil {
		a.logger.Debug(ctx, "coordinator exited", slog.Error(err))
		return xerrors.Errorf("run coordinator: %w", err)
	}
	return nil
}

func (a *agent) trackConnGoroutine(fn func()) error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return xerrors.New("track conn goroutine: agent is closed")
	}
	a.connCloseWait.Add(1)
	go func() {
		defer a.connCloseWait.Done()
		fn()
	}()
	return nil
}

func (a *agent) createTailnet(ctx context.Context, derpMap *tailcfg.DERPMap) (_ *tailnet.Conn, err error) {
	network, err := tailnet.NewConn(&tailnet.Options{
		Addresses:          []netip.Prefix{netip.PrefixFrom(codersdk.TailnetIP, 128)},
		DERPMap:            derpMap,
		Logger:             a.logger.Named("tailnet"),
		EnableTrafficStats: true,
	})
	if err != nil {
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	defer func() {
		if err != nil {
			network.Close()
		}
	}()

	sshListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetSSHPort))
	if err != nil {
		return nil, xerrors.Errorf("listen on the ssh port: %w", err)
	}
	defer func() {
		if err != nil {
			_ = sshListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		for {
			conn, err := sshListener.Accept()
			if err != nil {
				return
			}
			go a.sshServer.HandleConn(conn)
		}
	}); err != nil {
		return nil, err
	}

	reconnectingPTYListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetReconnectingPTYPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for reconnecting pty: %w", err)
	}
	defer func() {
		if err != nil {
			_ = reconnectingPTYListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		logger := a.logger.Named("reconnecting-pty")

		for {
			conn, err := reconnectingPTYListener.Accept()
			if err != nil {
				logger.Debug(ctx, "accept pty failed", slog.Error(err))
				return
			}
			// This cannot use a JSON decoder, since that can
			// buffer additional data that is required for the PTY.
			rawLen := make([]byte, 2)
			_, err = conn.Read(rawLen)
			if err != nil {
				continue
			}
			length := binary.LittleEndian.Uint16(rawLen)
			data := make([]byte, length)
			_, err = conn.Read(data)
			if err != nil {
				continue
			}
			var msg codersdk.ReconnectingPTYInit
			err = json.Unmarshal(data, &msg)
			if err != nil {
				continue
			}
			go func() {
				_ = a.handleReconnectingPTY(ctx, logger, msg, conn)
			}()
		}
	}); err != nil {
		return nil, err
	}

	speedtestListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetSpeedtestPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for speedtest: %w", err)
	}
	defer func() {
		if err != nil {
			_ = speedtestListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		for {
			conn, err := speedtestListener.Accept()
			if err != nil {
				a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				return
			}
			if err = a.trackConnGoroutine(func() {
				_ = speedtest.ServeConn(conn)
			}); err != nil {
				a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				_ = conn.Close()
				return
			}
		}
	}); err != nil {
		return nil, err
	}

	statisticsListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetStatisticsPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for statistics: %w", err)
	}
	defer func() {
		if err != nil {
			_ = statisticsListener.Close()
		}
	}()
	if err = a.trackConnGoroutine(func() {
		defer statisticsListener.Close()
		server := &http.Server{
			Handler:           a.statisticsHandler(),
			ReadTimeout:       20 * time.Second,
			ReadHeaderTimeout: 20 * time.Second,
			WriteTimeout:      20 * time.Second,
			ErrorLog:          slog.Stdlib(ctx, a.logger.Named("statistics_http_server"), slog.LevelInfo),
		}
		go func() {
			<-ctx.Done()
			_ = server.Close()
		}()

		err := server.Serve(statisticsListener)
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) && !strings.Contains(err.Error(), "use of closed network connection") {
			a.logger.Critical(ctx, "serve statistics HTTP server", slog.Error(err))
		}
	}); err != nil {
		return nil, err
	}

	return network, nil
}

// runCoordinator runs a coordinator and returns whether a reconnect
// should occur.
func (a *agent) runCoordinator(ctx context.Context, network *tailnet.Conn) error {
	coordinator, err := a.client.ListenWorkspaceAgent(ctx)
	if err != nil {
		return err
	}
	defer coordinator.Close()
	a.logger.Info(ctx, "connected to coordination server")
	sendNodes, errChan := tailnet.ServeCoordinator(coordinator, network.UpdateNodes)
	network.SetNodeCallback(sendNodes)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errChan:
		return err
	}
}

func (a *agent) runStartupScript(ctx context.Context, script string) error {
	if script == "" {
		return nil
	}

	a.logger.Info(ctx, "running startup script", slog.F("script", script))
	writer, err := a.filesystem.OpenFile(filepath.Join(a.tempDir, "coder-startup-script.log"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return xerrors.Errorf("open startup script log file: %w", err)
	}
	defer func() {
		_ = writer.Close()
	}()
	cmd, err := a.createCommand(ctx, script, nil)
	if err != nil {
		return xerrors.Errorf("create command: %w", err)
	}
	cmd.Stdout = writer
	cmd.Stderr = writer
	err = cmd.Run()
	if err != nil {
		// cmd.Run does not return a context canceled error, it returns "signal: killed".
		if ctx.Err() != nil {
			return ctx.Err()
		}

		return xerrors.Errorf("run: %w", err)
	}

	return nil
}

func (a *agent) init(ctx context.Context) {
	a.logger.Info(ctx, "generating host key")
	// Clients' should ignore the host key when connecting.
	// The agent needs to authenticate with coderd to SSH,
	// so SSH authentication doesn't improve security.
	randomHostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	randomSigner, err := gossh.NewSignerFromKey(randomHostKey)
	if err != nil {
		panic(err)
	}
	sshLogger := a.logger.Named("ssh-server")
	forwardHandler := &ssh.ForwardedTCPHandler{}
	a.sshServer = &ssh.Server{
		ChannelHandlers: map[string]ssh.ChannelHandler{
			"direct-tcpip": ssh.DirectTCPIPHandler,
			"session":      ssh.DefaultSessionHandler,
		},
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			sshLogger.Info(ctx, "ssh connection ended", slog.Error(err))
		},
		Handler: func(session ssh.Session) {
			err := a.handleSSHSession(session)
			var exitError *exec.ExitError
			if xerrors.As(err, &exitError) {
				a.logger.Debug(ctx, "ssh session returned", slog.Error(exitError))
				_ = session.Exit(exitError.ExitCode())
				return
			}
			if err != nil {
				a.logger.Warn(ctx, "ssh session failed", slog.Error(err))
				// This exit code is designed to be unlikely to be confused for a legit exit code
				// from the process.
				_ = session.Exit(MagicSessionErrorCode)
				return
			}
		},
		HostSigners: []ssh.Signer{randomSigner},
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			// Allow local port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("destination-host", destinationHost),
				slog.F("destination-port", destinationPort))
			return true
		},
		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return true
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			// Allow reverse port forwarding all!
			sshLogger.Debug(ctx, "local port forward",
				slog.F("bind-host", bindHost),
				slog.F("bind-port", bindPort))
			return true
		},
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
		},
		ServerConfigCallback: func(ctx ssh.Context) *gossh.ServerConfig {
			return &gossh.ServerConfig{
				NoClientAuth: true,
			}
		},
		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": func(session ssh.Session) {
				ctx := session.Context()

				// Typically sftp sessions don't request a TTY, but if they do,
				// we must ensure the gliderlabs/ssh CRLF emulation is disabled.
				// Otherwise sftp will be broken. This can happen if a user sets
				// `RequestTTY force` in their SSH config.
				session.DisablePTYEmulation()

				var opts []sftp.ServerOption
				// Change current working directory to the users home
				// directory so that SFTP connections land there.
				homedir, err := userHomeDir()
				if err != nil {
					sshLogger.Warn(ctx, "get sftp working directory failed, unable to get home dir", slog.Error(err))
				} else {
					opts = append(opts, sftp.WithServerWorkingDirectory(homedir))
				}

				server, err := sftp.NewServer(session, opts...)
				if err != nil {
					sshLogger.Debug(ctx, "initialize sftp server", slog.Error(err))
					return
				}
				defer server.Close()

				err = server.Serve()
				if errors.Is(err, io.EOF) {
					// Unless we call `session.Exit(0)` here, the client won't
					// receive `exit-status` because `(*sftp.Server).Close()`
					// calls `Close()` on the underlying connection (session),
					// which actually calls `channel.Close()` because it isn't
					// wrapped. This causes sftp clients to receive a non-zero
					// exit code. Typically sftp clients don't echo this exit
					// code but `scp` on macOS does (when using the default
					// SFTP backend).
					_ = session.Exit(0)
					return
				}
				sshLogger.Warn(ctx, "sftp server closed with error", slog.Error(err))
				_ = session.Exit(1)
			},
		},
	}

	go a.runLoop(ctx)
}

func convertAgentStats(counts map[netlogtype.Connection]netlogtype.Counts) *codersdk.AgentStats {
	stats := &codersdk.AgentStats{
		ConnsByProto: map[string]int64{},
		NumConns:     int64(len(counts)),
	}

	for conn, count := range counts {
		stats.ConnsByProto[conn.Proto.String()]++
		stats.RxPackets += int64(count.RxPackets)
		stats.RxBytes += int64(count.RxBytes)
		stats.TxPackets += int64(count.TxPackets)
		stats.TxBytes += int64(count.TxBytes)
	}

	return stats
}

// createCommand processes raw command input with OpenSSH-like behavior.
// If the rawCommand provided is empty, it will default to the users shell.
// This injects environment variables specified by the user at launch too.
func (a *agent) createCommand(ctx context.Context, rawCommand string, env []string) (*exec.Cmd, error) {
	currentUser, err := user.Current()
	if err != nil {
		return nil, xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return nil, xerrors.Errorf("get user shell: %w", err)
	}

	rawMetadata := a.metadata.Load()
	if rawMetadata == nil {
		return nil, xerrors.Errorf("no metadata was provided: %w", err)
	}
	metadata, valid := rawMetadata.(codersdk.WorkspaceAgentMetadata)
	if !valid {
		return nil, xerrors.Errorf("metadata is the wrong type: %T", metadata)
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	args := []string{caller, rawCommand}

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	if len(rawCommand) == 0 {
		args = []string{}
		if runtime.GOOS != "windows" {
			// On Linux and macOS, we should start a login
			// shell to consume juicy environment variables!
			args = append(args, "-l")
		}
	}

	cmd := exec.CommandContext(ctx, shell, args...)
	cmd.Dir = metadata.Directory
	if cmd.Dir == "" {
		// Default to user home if a directory is not set.
		homedir, err := userHomeDir()
		if err != nil {
			return nil, xerrors.Errorf("get home dir: %w", err)
		}
		cmd.Dir = homedir
	}
	cmd.Env = append(os.Environ(), env...)
	executablePath, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("getting os executable: %w", err)
	}
	// Set environment variables reliable detection of being inside a
	// Coder workspace.
	cmd.Env = append(cmd.Env, "CODER=true")
	cmd.Env = append(cmd.Env, fmt.Sprintf("USER=%s", username))
	// Git on Windows resolves with UNIX-style paths.
	// If using backslashes, it's unable to find the executable.
	unixExecutablePath := strings.ReplaceAll(executablePath, "\\", "/")
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, unixExecutablePath))

	// Specific Coder subcommands require the agent token exposed!
	cmd.Env = append(cmd.Env, fmt.Sprintf("CODER_AGENT_TOKEN=%s", *a.sessionToken.Load()))

	// Set SSH connection environment variables (these are also set by OpenSSH
	// and thus expected to be present by SSH clients). Since the agent does
	// networking in-memory, trying to provide accurate values here would be
	// nonsensical. For now, we hard code these values so that they're present.
	srcAddr, srcPort := "0.0.0.0", "0"
	dstAddr, dstPort := "0.0.0.0", "0"
	cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_CLIENT=%s %s %s", srcAddr, srcPort, dstPort))
	cmd.Env = append(cmd.Env, fmt.Sprintf("SSH_CONNECTION=%s %s %s %s", srcAddr, srcPort, dstAddr, dstPort))

	// This adds the ports dialog to code-server that enables
	// proxying a port dynamically.
	cmd.Env = append(cmd.Env, fmt.Sprintf("VSCODE_PROXY_URI=%s", metadata.VSCodePortProxyURI))

	// Hide Coder message on code-server's "Getting Started" page
	cmd.Env = append(cmd.Env, "CS_DISABLE_GETTING_STARTED_OVERRIDE=true")

	// Load environment variables passed via the agent.
	// These should override all variables we manually specify.
	for envKey, value := range metadata.EnvironmentVariables {
		// Expanding environment variables allows for customization
		// of the $PATH, among other variables. Customers can prepend
		// or append to the $PATH, so allowing expand is required!
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envKey, os.ExpandEnv(value)))
	}

	// Agent-level environment variables should take over all!
	// This is used for setting agent-specific variables like "CODER_AGENT_TOKEN".
	for envKey, value := range a.envVars {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", envKey, value))
	}

	return cmd, nil
}

func (a *agent) handleSSHSession(session ssh.Session) (retErr error) {
	ctx := session.Context()
	cmd, err := a.createCommand(ctx, session.RawCommand(), session.Environ())
	if err != nil {
		return err
	}

	if ssh.AgentRequested(session) {
		l, err := ssh.NewAgentListener()
		if err != nil {
			return xerrors.Errorf("new agent listener: %w", err)
		}
		defer l.Close()
		go ssh.ForwardAgentConnections(l, session)
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
	}

	sshPty, windowSize, isPty := session.Pty()
	if isPty {
		// Disable minimal PTY emulation set by gliderlabs/ssh (NL-to-CRNL).
		// See https://github.com/coder/coder/issues/3371.
		session.DisablePTYEmulation()

		if !isQuietLogin(session.RawCommand()) {
			metadata, ok := a.metadata.Load().(codersdk.WorkspaceAgentMetadata)
			if ok {
				err = showMOTD(session, metadata.MOTDFile)
				if err != nil {
					a.logger.Error(ctx, "show MOTD", slog.Error(err))
				}
			} else {
				a.logger.Warn(ctx, "metadata lookup failed, unable to show MOTD")
			}
		}

		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", sshPty.Term))

		// The pty package sets `SSH_TTY` on supported platforms.
		ptty, process, err := pty.Start(cmd, pty.WithPTYOption(
			pty.WithSSHRequest(sshPty),
			pty.WithLogger(slog.Stdlib(ctx, a.logger, slog.LevelInfo)),
		))
		if err != nil {
			return xerrors.Errorf("start command: %w", err)
		}
		defer func() {
			closeErr := ptty.Close()
			if closeErr != nil {
				a.logger.Warn(ctx, "failed to close tty", slog.Error(closeErr))
				if retErr == nil {
					retErr = closeErr
				}
			}
		}()
		go func() {
			for win := range windowSize {
				resizeErr := ptty.Resize(uint16(win.Height), uint16(win.Width))
				if resizeErr != nil {
					a.logger.Warn(ctx, "failed to resize tty", slog.Error(resizeErr))
				}
			}
		}()
		go func() {
			_, _ = io.Copy(ptty.Input(), session)
		}()
		go func() {
			_, _ = io.Copy(session, ptty.Output())
		}()
		err = process.Wait()
		var exitErr *exec.ExitError
		// ExitErrors just mean the command we run returned a non-zero exit code, which is normal
		// and not something to be concerned about.  But, if it's something else, we should log it.
		if err != nil && !xerrors.As(err, &exitErr) {
			a.logger.Warn(ctx, "wait error", slog.Error(err))
		}
		return err
	}

	cmd.Stdout = session
	cmd.Stderr = session.Stderr()
	// This blocks forever until stdin is received if we don't
	// use StdinPipe. It's unknown what causes this.
	stdinPipe, err := cmd.StdinPipe()
	if err != nil {
		return xerrors.Errorf("create stdin pipe: %w", err)
	}
	go func() {
		_, _ = io.Copy(stdinPipe, session)
		_ = stdinPipe.Close()
	}()
	err = cmd.Start()
	if err != nil {
		return xerrors.Errorf("start: %w", err)
	}
	return cmd.Wait()
}

func (a *agent) handleReconnectingPTY(ctx context.Context, logger slog.Logger, msg codersdk.ReconnectingPTYInit, conn net.Conn) (retErr error) {
	defer conn.Close()

	connectionID := uuid.NewString()
	logger = logger.With(slog.F("id", msg.ID), slog.F("connection_id", connectionID))

	defer func() {
		if err := retErr; err != nil {
			a.closeMutex.Lock()
			closed := a.isClosed()
			a.closeMutex.Unlock()

			// If the agent is closed, we don't want to
			// log this as an error since it's expected.
			if closed {
				logger.Debug(ctx, "session error after agent close", slog.Error(err))
			} else {
				logger.Error(ctx, "session error", slog.Error(err))
			}
		}
		logger.Debug(ctx, "session closed")
	}()

	var rpty *reconnectingPTY
	rawRPTY, ok := a.reconnectingPTYs.Load(msg.ID)
	if ok {
		logger.Debug(ctx, "connecting to existing session")
		rpty, ok = rawRPTY.(*reconnectingPTY)
		if !ok {
			return xerrors.Errorf("found invalid type in reconnecting pty map: %T", rawRPTY)
		}
	} else {
		logger.Debug(ctx, "creating new session")

		// Empty command will default to the users shell!
		cmd, err := a.createCommand(ctx, msg.Command, nil)
		if err != nil {
			return xerrors.Errorf("create command: %w", err)
		}
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")

		// Default to buffer 64KiB.
		circularBuffer, err := circbuf.NewBuffer(64 << 10)
		if err != nil {
			return xerrors.Errorf("create circular buffer: %w", err)
		}

		ptty, process, err := pty.Start(cmd)
		if err != nil {
			return xerrors.Errorf("start command: %w", err)
		}

		ctx, cancelFunc := context.WithCancel(ctx)
		rpty = &reconnectingPTY{
			activeConns: map[string]net.Conn{
				// We have to put the connection in the map instantly otherwise
				// the connection won't be closed if the process instantly dies.
				connectionID: conn,
			},
			ptty: ptty,
			// Timeouts created with an after func can be reset!
			timeout:        time.AfterFunc(a.reconnectingPTYTimeout, cancelFunc),
			circularBuffer: circularBuffer,
		}
		a.reconnectingPTYs.Store(msg.ID, rpty)
		go func() {
			// CommandContext isn't respected for Windows PTYs right now,
			// so we need to manually track the lifecycle.
			// When the context has been completed either:
			// 1. The timeout completed.
			// 2. The parent context was canceled.
			<-ctx.Done()
			_ = process.Kill()
		}()
		go func() {
			// If the process dies randomly, we should
			// close the pty.
			_ = process.Wait()
			rpty.Close()
		}()
		if err = a.trackConnGoroutine(func() {
			buffer := make([]byte, 1024)
			for {
				read, err := rpty.ptty.Output().Read(buffer)
				if err != nil {
					// When the PTY is closed, this is triggered.
					break
				}
				part := buffer[:read]
				rpty.circularBufferMutex.Lock()
				_, err = rpty.circularBuffer.Write(part)
				rpty.circularBufferMutex.Unlock()
				if err != nil {
					logger.Error(ctx, "write to circular buffer", slog.Error(err))
					break
				}
				rpty.activeConnsMutex.Lock()
				for _, conn := range rpty.activeConns {
					_, _ = conn.Write(part)
				}
				rpty.activeConnsMutex.Unlock()
			}

			// Cleanup the process, PTY, and delete it's
			// ID from memory.
			_ = process.Kill()
			rpty.Close()
			a.reconnectingPTYs.Delete(msg.ID)
		}); err != nil {
			return xerrors.Errorf("start routine: %w", err)
		}
	}
	// Resize the PTY to initial height + width.
	err := rpty.ptty.Resize(msg.Height, msg.Width)
	if err != nil {
		// We can continue after this, it's not fatal!
		logger.Error(ctx, "resize", slog.Error(err))
	}
	// Write any previously stored data for the TTY.
	rpty.circularBufferMutex.RLock()
	prevBuf := slices.Clone(rpty.circularBuffer.Bytes())
	rpty.circularBufferMutex.RUnlock()
	// Note that there is a small race here between writing buffered
	// data and storing conn in activeConns. This is likely a very minor
	// edge case, but we should look into ways to avoid it. Holding
	// activeConnsMutex would be one option, but holding this mutex
	// while also holding circularBufferMutex seems dangerous.
	_, err = conn.Write(prevBuf)
	if err != nil {
		return xerrors.Errorf("write buffer to conn: %w", err)
	}
	// Multiple connections to the same TTY are permitted.
	// This could easily be used for terminal sharing, but
	// we do it because it's a nice user experience to
	// copy/paste a terminal URL and have it _just work_.
	rpty.activeConnsMutex.Lock()
	rpty.activeConns[connectionID] = conn
	rpty.activeConnsMutex.Unlock()
	// Resetting this timeout prevents the PTY from exiting.
	rpty.timeout.Reset(a.reconnectingPTYTimeout)

	ctx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()
	heartbeat := time.NewTicker(a.reconnectingPTYTimeout / 2)
	defer heartbeat.Stop()
	go func() {
		// Keep updating the activity while this
		// connection is alive!
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
			}
			rpty.timeout.Reset(a.reconnectingPTYTimeout)
		}
	}()
	defer func() {
		// After this connection ends, remove it from
		// the PTYs active connections. If it isn't
		// removed, all PTY data will be sent to it.
		rpty.activeConnsMutex.Lock()
		delete(rpty.activeConns, connectionID)
		rpty.activeConnsMutex.Unlock()
	}()
	decoder := json.NewDecoder(conn)
	var req codersdk.ReconnectingPTYRequest
	for {
		err = decoder.Decode(&req)
		if xerrors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			logger.Warn(ctx, "read conn", slog.Error(err))
			return nil
		}
		_, err = rpty.ptty.Input().Write([]byte(req.Data))
		if err != nil {
			logger.Warn(ctx, "write to pty", slog.Error(err))
			return nil
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = rpty.ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			logger.Error(ctx, "resize", slog.Error(err))
		}
	}
}

// isClosed returns whether the API is closed or not.
func (a *agent) isClosed() bool {
	select {
	case <-a.closed:
		return true
	default:
		return false
	}
}

func (a *agent) Close() error {
	a.closeMutex.Lock()
	defer a.closeMutex.Unlock()
	if a.isClosed() {
		return nil
	}
	close(a.closed)
	a.closeCancel()
	if a.network != nil {
		_ = a.network.Close()
	}
	_ = a.sshServer.Close()
	a.connCloseWait.Wait()
	return nil
}

type reconnectingPTY struct {
	activeConnsMutex sync.Mutex
	activeConns      map[string]net.Conn

	circularBuffer      *circbuf.Buffer
	circularBufferMutex sync.RWMutex
	timeout             *time.Timer
	ptty                pty.PTY
}

// Close ends all connections to the reconnecting
// PTY and clear the circular buffer.
func (r *reconnectingPTY) Close() {
	r.activeConnsMutex.Lock()
	defer r.activeConnsMutex.Unlock()
	for _, conn := range r.activeConns {
		_ = conn.Close()
	}
	_ = r.ptty.Close()
	r.circularBufferMutex.Lock()
	r.circularBuffer.Reset()
	r.circularBufferMutex.Unlock()
	r.timeout.Stop()
}

// Bicopy copies all of the data between the two connections and will close them
// after one or both of them are done writing. If the context is canceled, both
// of the connections will be closed.
func Bicopy(ctx context.Context, c1, c2 io.ReadWriteCloser) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	defer func() {
		_ = c1.Close()
		_ = c2.Close()
	}()

	var wg sync.WaitGroup
	copyFunc := func(dst io.WriteCloser, src io.Reader) {
		defer func() {
			wg.Done()
			// If one side of the copy fails, ensure the other one exits as
			// well.
			cancel()
		}()
		_, _ = io.Copy(dst, src)
	}

	wg.Add(2)
	go copyFunc(c1, c2)
	go copyFunc(c2, c1)

	// Convert waitgroup to a channel so we can also wait on the context.
	done := make(chan struct{})
	go func() {
		defer close(done)
		wg.Wait()
	}()

	select {
	case <-ctx.Done():
	case <-done:
	}
}

// isQuietLogin checks if the SSH server should perform a quiet login or not.
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L816
func isQuietLogin(rawCommand string) bool {
	// We are always quiet unless this is a login shell.
	if len(rawCommand) != 0 {
		return true
	}

	// Best effort, if we can't get the home directory,
	// we can't lookup .hushlogin.
	homedir, err := userHomeDir()
	if err != nil {
		return false
	}

	_, err = os.Stat(filepath.Join(homedir, ".hushlogin"))
	return err == nil
}

// showMOTD will output the message of the day from
// the given filename to dest, if the file exists.
//
// https://github.com/openssh/openssh-portable/blob/25bd659cc72268f2858c5415740c442ee950049f/session.c#L784
func showMOTD(dest io.Writer, filename string) error {
	if filename == "" {
		return nil
	}

	f, err := os.Open(filename)
	if err != nil {
		if xerrors.Is(err, os.ErrNotExist) {
			// This is not an error, there simply isn't a MOTD to show.
			return nil
		}
		return xerrors.Errorf("open MOTD: %w", err)
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		// Carriage return ensures each line starts
		// at the beginning of the terminal.
		_, err = fmt.Fprint(dest, s.Text()+"\r\n")
		if err != nil {
			return xerrors.Errorf("write MOTD: %w", err)
		}
	}
	if err := s.Err(); err != nil {
		return xerrors.Errorf("read MOTD: %w", err)
	}

	return nil
}

// userHomeDir returns the home directory of the current user, giving
// priority to the $HOME environment variable.
func userHomeDir() (string, error) {
	// First we check the environment.
	homedir, err := os.UserHomeDir()
	if err == nil {
		return homedir, nil
	}

	// As a fallback, we try the user information.
	u, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("current user: %w", err)
	}
	return u.HomeDir, nil
}
