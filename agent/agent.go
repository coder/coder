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

func (s *agent) run(ctx context.Context) {
	var peerListener *peerbroker.Listener
	var err error
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		peerListener, err = s.clientDialer(ctx, s.options)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return
			}
			if s.isClosed() {
				return
			}
			s.options.Logger.Warn(context.Background(), "failed to dial", slog.Error(err))
			continue
		}
		s.options.Logger.Info(context.Background(), "connected")
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
			if s.isClosed() {
				return
			}
			s.options.Logger.Debug(ctx, "peer listener accept exited; restarting connection", slog.Error(err))
			s.run(ctx)
			return
		}
		s.closeMutex.Lock()
		s.connCloseWait.Add(1)
		s.closeMutex.Unlock()
		go s.handlePeerConn(ctx, conn)
	}
}

func (s *agent) handlePeerConn(ctx context.Context, conn *peer.Conn) {
	go func() {
		<-conn.Closed()
		s.connCloseWait.Done()
	}()
	for {
		channel, err := conn.Accept(ctx)
		if err != nil {
			if errors.Is(err, peer.ErrClosed) || s.isClosed() {
				return
			}
			s.options.Logger.Debug(ctx, "accept channel from peer connection", slog.Error(err))
			return
		}

		switch channel.Protocol() {
		case "ssh":
			s.sshServer.HandleConn(channel.NetConn())
		default:
			s.options.Logger.Warn(ctx, "unhandled protocol from channel",
				slog.F("protocol", channel.Protocol()),
				slog.F("label", channel.Label()),
			)
		}
	}
}

func (s *agent) init(ctx context.Context) {
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
	sshLogger := s.options.Logger.Named("ssh-server")
	forwardHandler := &ssh.ForwardedTCPHandler{}
	s.sshServer = &ssh.Server{
		ChannelHandlers: ssh.DefaultChannelHandlers,
		ConnectionFailedCallback: func(conn net.Conn, err error) {
			sshLogger.Info(ctx, "ssh connection ended", slog.Error(err))
		},
		Handler: func(session ssh.Session) {
			err := s.handleSSHSession(session)
			if err != nil {
				s.options.Logger.Debug(ctx, "ssh session failed", slog.Error(err))
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
				Config: gossh.Config{
					// "arcfour" is the fastest SSH cipher. We prioritize throughput
					// over encryption here, because the WebRTC connection is already
					// encrypted. If possible, we'd disable encryption entirely here.
					Ciphers: []string{"arcfour"},
				},
				NoClientAuth: true,
			}
		},
	}

	go s.run(ctx)
}

func (*agent) handleSSHSession(session ssh.Session) error {
	var (
		command string
		args    = []string{}
		err     error
	)

	username := session.User()
	if username == "" {
		currentUser, err := user.Current()
		if err != nil {
			return xerrors.Errorf("get current user: %w", err)
		}
		username = currentUser.Username
	}

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
				err := ptty.Resize(uint16(win.Width), uint16(win.Height))
				if err != nil {
					panic(err)
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
func (s *agent) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *agent) Close() error {
	s.closeMutex.Lock()
	defer s.closeMutex.Unlock()
	if s.isClosed() {
		return nil
	}
	close(s.closed)
	s.closeCancel()
	_ = s.sshServer.Close()
	s.connCloseWait.Wait()
	return nil
}
