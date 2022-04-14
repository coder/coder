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
	"sync"
	"time"

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
	Logger slog.Logger
}

type Dialer func(ctx context.Context, options *peer.ConnOptions) (*peerbroker.Listener, error)

func New(dialer Dialer, options *peer.ConnOptions) io.Closer {
	ctx, cancelFunc := context.WithCancel(context.Background())
	server := &agent{
		clientDialer: dialer,
		options:      options,
		closeCancel:  cancelFunc,
		closed:       make(chan struct{}),
	}
	server.init(ctx)
	return server
}

type agent struct {
	clientDialer Dialer
	options      *peer.ConnOptions

	connCloseWait sync.WaitGroup
	closeCancel   context.CancelFunc
	closeMutex    sync.Mutex
	closed        chan struct{}

	sshServer *ssh.Server
}

func (a *agent) run(ctx context.Context) {
	var peerListener *peerbroker.Listener
	var err error
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		peerListener, err = a.clientDialer(ctx, a.options)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if a.isClosed() {
				return
			}
			a.options.Logger.Warn(context.Background(), "failed to dial", slog.Error(err))
			continue
		}
		a.options.Logger.Info(context.Background(), "connected")
		break
	}
	select {
	case <-ctx.Done():
		return
	default:
	}

	for {
		conn, err := peerListener.Accept()
		if err != nil {
			if a.isClosed() {
				return
			}
			a.options.Logger.Debug(ctx, "peer listener accept exited; restarting connection", slog.Error(err))
			a.run(ctx)
			return
		}
		a.closeMutex.Lock()
		a.connCloseWait.Add(1)
		a.closeMutex.Unlock()
		go a.handlePeerConn(ctx, conn)
	}
}

func (a *agent) handlePeerConn(ctx context.Context, conn *peer.Conn) {
	go func() {
		select {
		case <-a.closed:
			_ = conn.Close()
		case <-conn.Closed():
		}
		<-conn.Closed()
		a.connCloseWait.Done()
	}()
	for {
		channel, err := conn.Accept(ctx)
		if err != nil {
			if errors.Is(err, peer.ErrClosed) || a.isClosed() {
				return
			}
			a.options.Logger.Debug(ctx, "accept channel from peer connection", slog.Error(err))
			return
		}

		switch channel.Protocol() {
		case "ssh":
			go a.sshServer.HandleConn(channel.NetConn())
		default:
			a.options.Logger.Warn(ctx, "unhandled protocol from channel",
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
	sshLogger := a.options.Logger.Named("ssh-server")
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
				a.options.Logger.Warn(ctx, "ssh session failed", slog.Error(err))
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
					a.options.Logger.Debug(session.Context(), "initialize sftp server", slog.Error(err))
					return
				}
				defer server.Close()
				err = server.Serve()
				if errors.Is(err, io.EOF) {
					return
				}
				a.options.Logger.Debug(session.Context(), "sftp server exited with error", slog.Error(err))
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
	cmd := exec.CommandContext(session.Context(), shell, "-c", command)
	cmd.Env = append(os.Environ(), session.Environ()...)
	executablePath, err := os.Executable()
	if err != nil {
		return xerrors.Errorf("getting os executable: %w", err)
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf(`GIT_SSH_COMMAND="%s gitssh --"`, executablePath))

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
					a.options.Logger.Warn(context.Background(), "failed to resize tty", slog.Error(err))
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
