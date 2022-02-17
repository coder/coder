package agent

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"io"
	"net"
	"os/exec"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
	"github.com/coder/coder/pty"
	"github.com/coder/retry"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

type Options struct {
	Logger slog.Logger
}

type Dialer func(ctx context.Context) (*peerbroker.Listener, error)

func Server(dialer Dialer, options *Options) io.Closer {
	ctx, cancelFunc := context.WithCancel(context.Background())
	s := &server{
		clientDialer: dialer,
		options:      options,
		closeCancel:  cancelFunc,
	}
	s.init(ctx)
	return s
}

type server struct {
	clientDialer Dialer
	options      *Options

	closeCancel context.CancelFunc
	closeMutex  sync.Mutex
	closed      chan struct{}
	closeError  error

	sshServer *ssh.Server
}

func (s *server) init(ctx context.Context) {
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
			_, windowSize, isPty := session.Pty()
			if isPty {
				pty, err := pty.Start(exec.Command("powershell.exe"))
				if err != nil {
					panic(err)
				}
				go func() {
					for win := range windowSize {
						err := pty.Resize(uint16(win.Width), uint16(win.Height))
						if err != nil {
							panic(err)
						}
					}
				}()
				go func() {
					io.Copy(session, pty.Output())
				}()
				io.Copy(pty.Input(), session)
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

func (s *server) run(ctx context.Context) {
	var peerListener *peerbroker.Listener
	var err error
	// An exponential back-off occurs when the connection is failing to dial.
	// This is to prevent server spam in case of a coderd outage.
	for retrier := retry.New(50*time.Millisecond, 10*time.Second); retrier.Wait(ctx); {
		peerListener, err = s.clientDialer(ctx)
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
		s.options.Logger.Debug(context.Background(), "connected")
		break
	}

	for {
		conn, err := peerListener.Accept()
		if err != nil {
			// This is closed!
			return
		}
		go s.handle(ctx, conn)
	}
}

func (s *server) handle(ctx context.Context, conn *peer.Conn) {
	for {
		channel, err := conn.Accept(ctx)
		if err != nil {
			// TODO: Log here!
			return
		}

		switch channel.Protocol() {
		case "ssh":
			s.sshServer.HandleConn(channel.NetConn())
		case "proxy":
			// Proxy the port provided.
		}
	}
}

// isClosed returns whether the API is closed or not.
func (s *server) isClosed() bool {
	select {
	case <-s.closed:
		return true
	default:
		return false
	}
}

func (s *server) Close() error {
	return nil
}
