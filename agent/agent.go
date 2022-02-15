package agent

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"

	"cdr.dev/slog"
	"github.com/coder/coder/peer"
	"github.com/coder/coder/peerbroker"
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
	forwardHandler := &ssh.ForwardedTCPHandler{}
	s.sshServer = &ssh.Server{
		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			return false
		},
		ReversePortForwardingCallback: func(ctx ssh.Context, bindHost string, bindPort uint32) bool {
			return false
		},
		PtyCallback: func(ctx ssh.Context, pty ssh.Pty) bool {
			return false
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
		RequestHandlers: map[string]ssh.RequestHandler{
			"tcpip-forward":        forwardHandler.HandleSSHRequest,
			"cancel-tcpip-forward": forwardHandler.HandleSSHRequest,
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
