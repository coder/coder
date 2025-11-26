//go:build !windows

package agentsocket

import (
	"context"
	"errors"
	"net"
	"sync"

	"golang.org/x/xerrors"

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
	drpcServer *drpcserver.Server
	service    *DRPCAgentSocketService

	mu       sync.Mutex
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewServer creates a new agent socket server.
func NewServer(logger slog.Logger, opts ...Option) (*Server, error) {
	options := &options{}
	for _, opt := range opts {
		opt(options)
	}

	path := options.path
	if path == "" {
		path = defaultSocketPath
	}

	logger = logger.Named("agentsocket-server")
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

	listener, err := createSocket(server.path)
	if err != nil {
		return nil, xerrors.Errorf("create socket: %w", err)
	}

	server.listener = listener

	// This context is canceled by server.Close().
	// canceling it will close all connections.
	server.ctx, server.cancel = context.WithCancel(context.Background())

	server.logger.Info(server.ctx, "agent socket server started", slog.F("path", server.path))

	server.wg.Add(1)
	go func() {
		defer server.wg.Done()
		server.acceptConnections()
	}()

	return server, nil
}

// Close stops the server and cleans up resources.
func (s *Server) Close() error {
	s.mu.Lock()

	if s.listener == nil {
		s.mu.Unlock()
		return nil
	}

	s.logger.Info(s.ctx, "stopping agent socket server")

	s.cancel()

	if err := s.listener.Close(); err != nil {
		s.logger.Warn(s.ctx, "error closing socket listener", slog.Error(err))
	}

	s.listener = nil

	s.mu.Unlock()

	// Wait for all connections to finish
	s.wg.Wait()

	if err := cleanupSocket(s.path); err != nil {
		s.logger.Warn(s.ctx, "error cleaning up socket file", slog.Error(err))
	}

	s.logger.Info(s.ctx, "agent socket server stopped")

	return nil
}

func (s *Server) acceptConnections() {
	// In an edge case, Close() might race with acceptConnections() and set s.listener to nil.
	// Therefore, we grab a copy of the listener under a lock. We might still get a nil listener,
	// but then we know close has already run and we can return early.
	s.mu.Lock()
	listener := s.listener
	s.mu.Unlock()
	if listener == nil {
		return
	}

	err := s.drpcServer.Serve(s.ctx, listener)
	if err != nil {
		s.logger.Warn(s.ctx, "error serving drpc server", slog.Error(err))
	}
}
