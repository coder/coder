package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/armon/circbuf"
	"github.com/google/uuid"

	gsyslog "github.com/hashicorp/go-syslog"
	"go.uber.org/atomic"

	"cdr.dev/slog"
	"github.com/coder/coder/agent/usershell"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/pty"
	"github.com/coder/retry"

	"github.com/pkg/sftp"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/xerrors"
)

type Options struct {
	ReconnectingPTYTimeout time.Duration
	Logger                 slog.Logger
}

type Metadata struct {
	OwnerEmail           string            `json:"owner_email"`
	OwnerUsername        string            `json:"owner_username"`
	EnvironmentVariables map[string]string `json:"environment_variables"`
	StartupScript        string            `json:"startup_script"`
}

type Dialer func(ctx context.Context, logger slog.Logger) (Metadata, *peerbroker.Listener, error)

func New(dialer Dialer, options *Options) io.Closer {
	if options == nil {
		options = &Options{}
	}
	if options.ReconnectingPTYTimeout == 0 {
		options.ReconnectingPTYTimeout = 5 * time.Minute
	}
	ctx, cancelFunc := context.WithCancel(context.Background())
	server := &agent{
		dialer:                 dialer,
		reconnectingPTYTimeout: options.ReconnectingPTYTimeout,
		logger:                 options.Logger,
		closeCancel:            cancelFunc,
		closed:                 make(chan struct{}),
	}
	server.init(ctx)
	return server
}

type agent struct {
	dialer Dialer
	logger slog.Logger

	reconnectingPTYs       sync.Map
	reconnectingPTYTimeout time.Duration

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	// Environment variables sent by Coder to inject for shell sessions.
	// These are atomic because values can change after reconnect.
	envVars       atomic.Value
	ownerEmail    atomic.String
	ownerUsername atomic.String
	startupScript atomic.Bool
	sshServer     *ssh.Server
}

func (a *agent) run(ctx context.Context) {
	var options Metadata
	var peerListener *peerbroker.Listener
	var err error
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		options, peerListener, err = a.dialer(ctx, a.logger)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if a.isClosed() {
				return
			}
			a.logger.Warn(context.Background(), "failed to dial", slog.Error(err))
			continue
		}
		a.logger.Info(context.Background(), "connected")
		break
	}
	select {
	case <-ctx.Done():
		return
	default:
	}
	a.envVars.Store(options.EnvironmentVariables)
	a.ownerEmail.Store(options.OwnerEmail)
	a.ownerUsername.Store(options.OwnerUsername)

	if a.startupScript.CAS(false, true) {
		// The startup script has not ran yet!
		go func() {
			err := a.runStartupScript(ctx, options.StartupScript)
			if errors.Is(err, context.Canceled) {
				return
			}
			if err != nil {
				a.logger.Warn(ctx, "agent script failed", slog.Error(err))
			}
		}()
	}

	for {
		conn, err := peerListener.Accept()
		if err != nil {
			if a.isClosed() {
				return
			}
			a.logger.Debug(ctx, "peer listener accept exited; restarting connection", slog.Error(err))
			a.run(ctx)
			return
		}
		a.closeMutex.Lock()
		a.connCloseWait.Add(1)
		a.closeMutex.Unlock()
		go a.handlePeerConn(ctx, conn)
	}
}

func (*agent) runStartupScript(ctx context.Context, script string) error {
	if script == "" {
		return nil
	}
	currentUser, err := user.Current()
	if err != nil {
		return xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return xerrors.Errorf("get user shell: %w", err)
	}

	var writer io.WriteCloser
	// Attempt to use the syslog to write startup information.
	writer, err = gsyslog.NewLogger(gsyslog.LOG_INFO, "USER", "coder-startup-script")
	if err != nil {
		// If the syslog isn't supported or cannot be created, use a text file in temp.
		writer, err = os.CreateTemp("", "coder-startup-script.txt")
		if err != nil {
			return xerrors.Errorf("open startup script log file: %w", err)
		}
	}
	defer func() {
		_ = writer.Close()
	}()
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	cmd := exec.CommandContext(ctx, shell, caller, script)
	cmd.Stdout = writer
	cmd.Stderr = writer
	err = cmd.Run()
	if err != nil {
		return xerrors.Errorf("run: %w", err)
	}
	return nil
}

func (a *agent) handlePeerConn(ctx context.Context, conn *peer.Conn) {
	go func() {
		select {
		case <-a.closed:
		case <-conn.Closed():
		}
		_ = conn.Close()
		a.connCloseWait.Done()
	}()
	for {
		channel, err := conn.Accept(ctx)
		if err != nil {
			if errors.Is(err, peer.ErrClosed) || a.isClosed() {
				return
			}
			a.logger.Debug(ctx, "accept channel from peer connection", slog.Error(err))
			return
		}

		switch channel.Protocol() {
		case "ssh":
			go a.sshServer.HandleConn(channel.NetConn())
		case "reconnecting-pty":
			go a.handleReconnectingPTY(ctx, channel.Label(), channel.NetConn())
		default:
			a.logger.Warn(ctx, "unhandled protocol from channel",
				slog.F("protocol", channel.Protocol()),
				slog.F("label", channel.Label()),
			)
		}
	}
}

func (a *agent) init(ctx context.Context) {
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
			if err != nil {
				a.logger.Warn(ctx, "ssh session failed", slog.Error(err))
				_ = session.Exit(1)
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
				server, err := sftp.NewServer(session)
				if err != nil {
					a.logger.Debug(session.Context(), "initialize sftp server", slog.Error(err))
					return
				}
				defer server.Close()
				err = server.Serve()
				if errors.Is(err, io.EOF) {
					return
				}
				a.logger.Debug(session.Context(), "sftp server exited with error", slog.Error(err))
			},
		},
	}

	go a.run(ctx)
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

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	command := rawCommand
	if len(command) == 0 {
		command = shell
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	cmd := exec.CommandContext(ctx, shell, caller, command)
	cmd.Env = append(os.Environ(), env...)
	executablePath, err := os.Executable()
	if err != nil {
		return nil, xerrors.Errorf("getting os executable: %w", err)
	}
	// Git on Windows resolves with UNIX-style paths.
	// If using backslashes, it's unable to find the executable.
	executablePath = strings.ReplaceAll(executablePath, "\\", "/")
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, executablePath))
	// These prevent the user from having to specify _anything_ to successfully commit.
	// Both author and committer must be set!
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_AUTHOR_EMAIL=%s`, a.ownerEmail.Load()))
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_COMMITTER_EMAIL=%s`, a.ownerEmail.Load()))
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_AUTHOR_NAME=%s`, a.ownerUsername.Load()))
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_COMMITTER_NAME=%s`, a.ownerUsername.Load()))

	// Load environment variables passed via the agent.
	// These should override all variables we manually specify.
	envVars := a.envVars.Load()
	if envVars != nil {
		envVarMap, ok := envVars.(map[string]string)
		if ok {
			for key, value := range envVarMap {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
			}
		}
	}
	return cmd, nil
}

func (a *agent) handleSSHSession(session ssh.Session) error {
	cmd, err := a.createCommand(session.Context(), session.RawCommand(), session.Environ())
	if err != nil {
		return err
	}

	sshPty, windowSize, isPty := session.Pty()
	if isPty {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", sshPty.Term))
		ptty, process, err := pty.Start(cmd)
		if err != nil {
			return xerrors.Errorf("start command: %w", err)
		}
		err = ptty.Resize(uint16(sshPty.Window.Height), uint16(sshPty.Window.Width))
		if err != nil {
			return xerrors.Errorf("resize ptty: %w", err)
		}
		go func() {
			for win := range windowSize {
				err = ptty.Resize(uint16(win.Height), uint16(win.Width))
				if err != nil {
					a.logger.Warn(context.Background(), "failed to resize tty", slog.Error(err))
				}
			}
		}()
		go func() {
			_, _ = io.Copy(ptty.Input(), session)
		}()
		go func() {
			_, _ = io.Copy(session, ptty.Output())
		}()
		_, _ = process.Wait()
		_ = ptty.Close()
		return nil
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
	}()
	err = cmd.Start()
	if err != nil {
		return xerrors.Errorf("start: %w", err)
	}
	return cmd.Wait()
}

func (a *agent) handleReconnectingPTY(ctx context.Context, rawID string, conn net.Conn) {
	defer conn.Close()

	// The ID format is referenced in conn.go.
	// <uuid>:<height>:<width>
	idParts := strings.Split(rawID, ":")
	if len(idParts) != 3 {
		a.logger.Warn(ctx, "client sent invalid id format", slog.F("raw-id", rawID))
		return
	}
	id := idParts[0]
	// Enforce a consistent format for IDs.
	_, err := uuid.Parse(id)
	if err != nil {
		a.logger.Warn(ctx, "client sent reconnection token that isn't a uuid", slog.F("id", id), slog.Error(err))
		return
	}
	// Parse the initial terminal dimensions.
	height, err := strconv.Atoi(idParts[1])
	if err != nil {
		a.logger.Warn(ctx, "client sent invalid height", slog.F("id", id), slog.F("height", idParts[1]))
		return
	}
	width, err := strconv.Atoi(idParts[2])
	if err != nil {
		a.logger.Warn(ctx, "client sent invalid width", slog.F("id", id), slog.F("width", idParts[2]))
		return
	}

	var rpty *reconnectingPTY
	rawRPTY, ok := a.reconnectingPTYs.Load(id)
	if ok {
		rpty, ok = rawRPTY.(*reconnectingPTY)
		if !ok {
			a.logger.Warn(ctx, "found invalid type in reconnecting pty map", slog.F("id", id))
		}
	} else {
		// Empty command will default to the users shell!
		cmd, err := a.createCommand(ctx, "", nil)
		if err != nil {
			a.logger.Warn(ctx, "create reconnecting pty command", slog.Error(err))
			return
		}
		cmd.Env = append(cmd.Env, "TERM=xterm-256color")

		ptty, process, err := pty.Start(cmd)
		if err != nil {
			a.logger.Warn(ctx, "start reconnecting pty command", slog.F("id", id))
		}

		// Default to buffer 64KB.
		circularBuffer, err := circbuf.NewBuffer(64 * 1024)
		if err != nil {
			a.logger.Warn(ctx, "create circular buffer", slog.Error(err))
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
		a.reconnectingPTYs.Store(id, rpty)
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
			_, _ = process.Wait()
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
				_, err = rpty.circularBuffer.Write(part)
				if err != nil {
					a.logger.Error(ctx, "reconnecting pty write buffer", slog.Error(err), slog.F("id", id))
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
			a.reconnectingPTYs.Delete(id)
			a.connCloseWait.Done()
		}()
	}
	// Resize the PTY to initial height + width.
	err = rpty.ptty.Resize(uint16(height), uint16(width))
	if err != nil {
		// We can continue after this, it's not fatal!
		a.logger.Error(ctx, "resize reconnecting pty", slog.F("id", id), slog.Error(err))
	}
	// Write any previously stored data for the TTY.
	_, err = conn.Write(rpty.circularBuffer.Bytes())
	if err != nil {
		a.logger.Warn(ctx, "write reconnecting pty buffer", slog.F("id", id), slog.Error(err))
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
	var req ReconnectingPTYRequest
	for {
		err = decoder.Decode(&req)
		if xerrors.Is(err, io.EOF) {
			return
		}
		if err != nil {
			a.logger.Warn(ctx, "reconnecting pty buffer read error", slog.F("id", id), slog.Error(err))
			return
		}
		_, err = rpty.ptty.Input().Write([]byte(req.Data))
		if err != nil {
			a.logger.Warn(ctx, "write to reconnecting pty", slog.F("id", id), slog.Error(err))
			return
		}
		// Check if a resize needs to happen!
		if req.Height == 0 || req.Width == 0 {
			continue
		}
		err = rpty.ptty.Resize(req.Height, req.Width)
		if err != nil {
			// We can continue after this, it's not fatal!
			a.logger.Error(ctx, "resize reconnecting pty", slog.F("id", id), slog.Error(err))
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
	_ = a.sshServer.Close()
	a.connCloseWait.Wait()
	return nil
}

type reconnectingPTY struct {
	activeConnsMutex sync.Mutex
	activeConns      map[string]net.Conn

	circularBuffer *circbuf.Buffer
	timeout        *time.Timer
	ptty           pty.PTY
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
	r.circularBuffer.Reset()
	r.timeout.Stop()
}
