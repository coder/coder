package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"sync"
	"time"

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
	EnvironmentVariables map[string]string
	StartupScript        string
}

type Dialer func(ctx context.Context, logger slog.Logger) (*Options, *peerbroker.Listener, error)

func New(dialer Dialer, logger slog.Logger) io.Closer {
	ctx, cancelFunc := context.WithCancel(context.Background())
	server := &agent{
		dialer:      dialer,
		logger:      logger,
		closeCancel: cancelFunc,
		closed:      make(chan struct{}),
	}
	server.init(ctx)
	return server
}

type agent struct {
	dialer Dialer
	logger slog.Logger

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	// Environment variables sent by Coder to inject for shell sessions.
	// This is atomic because values can change after reconnect.
	envVars       atomic.Value
	startupScript atomic.Bool
	sshServer     *ssh.Server
}

func (a *agent) run(ctx context.Context) {
	var options *Options
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

func (a *agent) handleSSHSession(session ssh.Session) error {
	currentUser, err := user.Current()
	if err != nil {
		return xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	shell, err := usershell.Get(username)
	if err != nil {
		return xerrors.Errorf("get user shell: %w", err)
	}

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	command := session.RawCommand()
	if len(session.Command()) == 0 {
		command = shell
	}

	// OpenSSH executes all commands with the users current shell.
	// We replicate that behavior for IDE support.
	caller := "-c"
	if runtime.GOOS == "windows" {
		caller = "/c"
	}
	cmd := exec.CommandContext(session.Context(), shell, caller, command)
	cmd.Env = append(os.Environ(), session.Environ()...)

	// Load environment variables passed via the agent.
	envVars := a.envVars.Load()
	if envVars != nil {
		envVarMap, ok := envVars.(map[string]string)
		if ok {
			for key, value := range envVarMap {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", key, value))
			}
		}
	}

	executablePath, err := os.Executable()
	if err != nil {
		return xerrors.Errorf("getting os executable: %w", err)
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_SSH_COMMAND=%s gitssh --`, executablePath))

	sshPty, windowSize, isPty := session.Pty()
	if isPty {
		cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", sshPty.Term))
		ptty, process, err := pty.Start(cmd)
		if err != nil {
			return xerrors.Errorf("start command: %w", err)
		}
		go func() {
			for win := range windowSize {
				err = ptty.Resize(uint16(win.Width), uint16(win.Height))
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
