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
			a.sshServer.HandleConn(channel.NetConn())
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
		ChannelHandlers: ssh.DefaultChannelHandlers,
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
	}

	go a.run(ctx)
}

func (a *agent) handleSSHSession(session ssh.Session) error {
	var (
		command string
		args    = []string{}
		err     error
	)

	currentUser, err := user.Current()
	if err != nil {
		return xerrors.Errorf("get current user: %w", err)
	}
	username := currentUser.Username

	// gliderlabs/ssh returns a command slice of zero
	// when a shell is requested.
	if len(session.Command()) == 0 {
		command, err = usershell.Get(username)
		if err != nil {
			return xerrors.Errorf("get user shell: %w", err)
		}
	} else {
		command = session.Command()[0]
		if len(session.Command()) > 1 {
			args = session.Command()[1:]
		}
	}

	signals := make(chan ssh.Signal)
	breaks := make(chan bool)
	defer close(signals)
	defer close(breaks)
	go func() {
		for {
			select {
			case <-session.Context().Done():
				return
			// Ignore signals and breaks for now!
			case <-signals:
			case <-breaks:
			}
		}
	}()

	cmd := exec.CommandContext(session.Context(), command, args...)
	cmd.Env = append(os.Environ(), session.Environ()...)

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
	cmd.Stderr = session
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
	_ = cmd.Wait()
	return nil
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
