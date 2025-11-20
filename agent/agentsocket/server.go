package agentsocket

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"github.com/hashicorp/yamux"
	"storj.io/drpc/drpcmux"
	"storj.io/drpc/drpcserver"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/agent/agentsocket/proto"
	"github.com/coder/coder/v2/agent/unit"
	"github.com/coder/coder/v2/codersdk/drpcsdk"
)

// Server provides access to the DRPCAgentSocketService via a Unix domain socket.
// Do not invoke Server{} directly. Use NewServer() instead.
type Server struct {
	logger     slog.Logger
	path       string
	listener   net.Listener
	mu         sync.Mutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	drpcServer *drpcserver.Server
	service    *DRPCAgentSocketService
}

func NewServer(path string, logger slog.Logger) (*Server, error) {
	logger = logger.Named("agentsocket")
	server := &Server{
		logger: logger,
		path:   path,
		service: &DRPCAgentSocketService{
			logger:      logger,
			unitManager: unit.NewManager(),
		},
	}

	mux := drpcmux.New()
	err := proto.DRPCRegisterAgentSocket(mux, server.service)
	if err != nil {
		return nil, xerrors.Errorf("failed to register drpc service: %w", err)
	}

	server.drpcServer = drpcserver.NewWithOptions(mux, drpcserver.Options{
		Manager: drpcsdk.DefaultDRPCOptions(nil),
		Log: func(err error) {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return
			}
			logger.Debug(context.Background(), "drpc server error", slog.Error(err))
		},
	})

	return server, nil
}

var ErrServerAlreadyStarted = xerrors.New("server already started")

func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return ErrServerAlreadyStarted
	}

	// This context is canceled by s.Stop() when the server is stopped.
	// canceling it will close all connections.
	s.ctx, s.cancel = context.WithCancel(context.Background())

	if s.path == "" {
		var err error
		s.path, err = getDefaultSocketPath()
		if err != nil {
			return xerrors.Errorf("get default socket path: %w", err)
		}
	}

	listener, err := createSocket(s.path)
	if err != nil {
		return xerrors.Errorf("create socket: %w", err)
	}

	s.listener = listener

	s.logger.Info(s.ctx, "agent socket server started", slog.F("path", s.path))

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.acceptConnections()
	}()

	return nil
}

func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return nil
	}

	s.logger.Info(s.ctx, "stopping agent socket server")

	s.cancel()

	if err := s.listener.Close(); err != nil {
		s.logger.Warn(s.ctx, "error closing socket listener", slog.Error(err))
	}

	// Wait for all connections to finish
	s.wg.Wait()

	if err := cleanupSocket(s.path); err != nil {
		s.logger.Warn(s.ctx, "error cleaning up socket file", slog.Error(err))
	}

	s.listener = nil
	s.logger.Info(s.ctx, "agent socket server stopped")

	return nil
}

func (s *Server) acceptConnections() {
	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
				s.logger.Warn(s.ctx, "error accepting connection", slog.Error(err))
				continue
			}
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		s.logger.Warn(s.ctx, "failed to set connection deadline", slog.Error(err))
	}

	s.logger.Debug(s.ctx, "new connection accepted", slog.F("remote_addr", conn.RemoteAddr()))

	config := yamux.DefaultConfig()
	config.Logger = nil
	session, err := yamux.Server(conn, config)
	if err != nil {
		s.logger.Warn(s.ctx, "failed to create yamux session", slog.Error(err))
		return
	}
	defer session.Close()

	err = s.drpcServer.Serve(s.ctx, session)
	if err != nil {
		s.logger.Debug(s.ctx, "drpc server finished", slog.Error(err))
	}
}
