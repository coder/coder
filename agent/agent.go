package agent

import (
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
	"golang.org/x/xerrors"
	"tailscale.com/net/speedtest"
	"tailscale.com/tailcfg"

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
	server := &agent{
		reconnectingPTYTimeout: options.ReconnectingPTYTimeout,
		logger:                 options.Logger,
		closeCancel:            cancelFunc,
		closed:                 make(chan struct{}),
		envVars:                options.EnvironmentVariables,
		client:                 options.Client,
		exchangeToken:          options.ExchangeToken,
		filesystem:             options.Filesystem,
		tempDir:                options.TempDir,
		stats:                  &Stats{},
	}
	server.init(ctx)
	return server
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
	stats   *Stats
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
		a.network = network
		a.closeMutex.Unlock()
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

func (a *agent) createTailnet(ctx context.Context, derpMap *tailcfg.DERPMap) (*tailnet.Conn, error) {
	a.closeMutex.Lock()
	if a.isClosed() {
		a.closeMutex.Unlock()
		return nil, xerrors.New("closed")
	}
	network, err := tailnet.NewConn(&tailnet.Options{
		Addresses: []netip.Prefix{netip.PrefixFrom(codersdk.TailnetIP, 128)},
		DERPMap:   derpMap,
		Logger:    a.logger.Named("tailnet"),
	})
	if err != nil {
		a.closeMutex.Unlock()
		return nil, xerrors.Errorf("create tailnet: %w", err)
	}
	a.network = network
	network.SetForwardTCPCallback(func(conn net.Conn, listenerExists bool) net.Conn {
		if listenerExists {
			// If a listener already exists, we would double-wrap the conn.
			return conn
		}
		return a.stats.wrapConn(conn)
	})
	a.connCloseWait.Add(4)
	a.closeMutex.Unlock()

	sshListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetSSHPort))
	if err != nil {
		return nil, xerrors.Errorf("listen on the ssh port: %w", err)
	}
	go func() {
		defer a.connCloseWait.Done()
		for {
			conn, err := sshListener.Accept()
			if err != nil {
				return
			}
			go a.sshServer.HandleConn(a.stats.wrapConn(conn))
		}
	}()

	reconnectingPTYListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetReconnectingPTYPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for reconnecting pty: %w", err)
	}
	go func() {
		defer a.connCloseWait.Done()
		for {
			conn, err := reconnectingPTYListener.Accept()
			if err != nil {
				a.logger.Debug(ctx, "accept pty failed", slog.Error(err))
				return
			}
			conn = a.stats.wrapConn(conn)
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
			go a.handleReconnectingPTY(ctx, msg, conn)
		}
	}()

	speedtestListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetSpeedtestPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for speedtest: %w", err)
	}
	go func() {
		defer a.connCloseWait.Done()
		for {
			conn, err := speedtestListener.Accept()
			if err != nil {
				a.logger.Debug(ctx, "speedtest listener failed", slog.Error(err))
				return
			}
			a.closeMutex.Lock()
			a.connCloseWait.Add(1)
			a.closeMutex.Unlock()
			go func() {
				defer a.connCloseWait.Done()
				_ = speedtest.ServeConn(conn)
			}()
		}
	}()

	statisticsListener, err := network.Listen("tcp", ":"+strconv.Itoa(codersdk.TailnetStatisticsPort))
	if err != nil {
		return nil, xerrors.Errorf("listen for statistics: %w", err)
	}
	go func() {
		defer a.connCloseWait.Done()
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

		err = server.Serve(statisticsListener)
		if err != nil && !xerrors.Is(err, http.ErrServerClosed) && !strings.Contains(err.Error(), "use of closed network connection") {
			a.logger.Critical(ctx, "serve statistics HTTP server", slog.Error(err))
		}
	}()

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
				// https://github.com/coder/coder/issues/3620
				u, err := user.Current()
				if err != nil {
					sshLogger.Warn(ctx, "get sftp working directory failed, unable to get current user", slog.Error(err))
				} else {
					opts = append(opts, sftp.WithServerWorkingDirectory(u.HomeDir))
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
	cl, err := a.client.AgentReportStats(ctx, a.logger, func() *codersdk.AgentStats {
		return a.stats.Copy()
	})
	if err != nil {
		a.logger.Error(ctx, "report stats", slog.Error(err))
		return
	}
	a.connCloseWait.Add(1)
	go func() {
		defer a.connCloseWait.Done()
		<-a.closed
		cl.Close()
	}()
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
		// Default to $HOME if a directory is not set!
		cmd.Dir = os.Getenv("HOME")
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

func (a *agent) handleReconnectingPTY(ctx context.Context, msg codersdk.ReconnectingPTYInit, conn net.Conn) {
	defer conn.Close()

	var rpty *reconnectingPTY
	rawRPTY, ok := a.reconnectingPTYs.Load(msg.ID)
	if ok {
		rpty, ok = rawRPTY.(*reconnectingPTY)
		if !ok {
			a.logger.Error(ctx, "found invalid type in reconnecting pty map", slog.F("id", msg.ID))
			return
		}
	} else {
		// Empty command will default to the users shell!
		cmd, err := a.createCommand(ctx, msg.Command, nil)
		if err != nil {
			a.logger.Error(ctx, "create reconnecting pty command", slog.Error(err))
			return
		}
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")

		// Default to buffer 64KiB.
		circularBuffer, err := circbuf.NewBuffer(64 << 10)
		if err != nil {
			a.logger.Error(ctx, "create circular buffer", slog.Error(err))
			return
		}

		ptty, process, err := pty.Start(cmd)
		if err != nil {
			a.logger.Error(ctx, "start reconnecting pty command", slog.F("id", msg.ID), slog.Error(err))
			return
		}

		a.closeMutex.Lock()
		a.connCloseWait.Add(1)
		a.closeMutex.Unlock()
		ctx, cancelFunc := context.WithCancel(ctx)
		rpty = &reconnectingPTY{
			activeConns: make(map[string]net.Conn),
			ptty:        ptty,
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
		go func() {
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
					a.logger.Error(ctx, "reconnecting pty write buffer", slog.Error(err), slog.F("id", msg.ID))
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
			a.connCloseWait.Done()
		}()
	}
	// Resize the PTY to initial height + width.
	err := rpty.ptty.Resize(msg.Height, msg.Width)
	if err != nil {
		// We can continue after this, it's not fatal!
		a.logger.Error(ctx, "resize reconnecting pty", slog.F("id", msg.ID), slog.Error(err))
	}
	// Write any previously stored data for the TTY.
	rpty.circularBufferMutex.RLock()
	_, err = conn.Write(rpty.circularBuffer.Bytes())
	rpty.circularBufferMutex.RUnlock()
	if err != nil {
		a.logger.Warn(ctx, "write reconnecting pty buffer", slog.F("id", msg.ID), slog.Error(err))
		return
	}
	connectionID := uuid.NewString()
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
			return
		}
		if err != nil {
			a.logger.Warn(ctx, "reconnecting pty buffer read error", slog.F("id", msg.ID), slog.Error(err))
			return
		}
		_, err = rpty.ptty.Input().Write([]byte(req.Data))
		if err != nil {
			a.logger.Warn(ctx, "write to reconnecting pty", slog.F("id", msg.ID), slog.Error(err))
			return
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = rpty.ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			a.logger.Error(ctx, "resize reconnecting pty", slog.F("id", msg.ID), slog.Error(err))
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

// ExpandRelativeHomePath expands the tilde at the beginning of a path to the
// current user's home directory and returns a full absolute path.
func ExpandRelativeHomePath(in string) (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("get current user details: %w", err)
	}

	if in == "~" {
		in = usr.HomeDir
	} else if strings.HasPrefix(in, "~/") {
		in = filepath.Join(usr.HomeDir, in[2:])
	}

	return filepath.Abs(in)
}
