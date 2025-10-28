package agentsocket

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"sync"
	"time"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

// Server represents the agent socket server
type Server struct {
	logger         slog.Logger
	path           string
	listener       net.Listener
	handlers       map[string]Handler
	authMiddleware AuthMiddleware
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	wg             sync.WaitGroup
}

// Config holds configuration for the socket server
type Config struct {
	Path           string
	Logger         slog.Logger
	AuthMiddleware AuthMiddleware
}

// NewServer creates a new agent socket server
func NewServer(config Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	server := &Server{
		logger:         config.Logger.Named("agentsocket"),
		path:           config.Path,
		handlers:       make(map[string]Handler),
		authMiddleware: config.AuthMiddleware,
		ctx:            ctx,
		cancel:         cancel,
	}

	// Set default auth middleware if none provided
	if server.authMiddleware == nil {
		server.authMiddleware = &NoAuthMiddleware{}
	}

	return server
}

// Start starts the socket server
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return xerrors.New("server already started")
	}

	// Get socket path
	path := s.path
	if path == "" {
		var err error
		path, err = getDefaultSocketPath()
		if err != nil {
			return xerrors.Errorf("get default socket path: %w", err)
		}
	}

	// Check if socket is available
	s.logger.Debug(s.ctx, "SOCKET_DEBUG: Checking if socket path is available", slog.F("path", path))
	if !isSocketAvailable(path, s.logger) {
		s.logger.Error(s.ctx, "SOCKET_DEBUG: Socket path is not available", slog.F("path", path))
		return xerrors.Errorf("socket path %s is not available", path)
	}
	s.logger.Debug(s.ctx, "SOCKET_DEBUG: Socket path is available", slog.F("path", path))

	// Create socket listener
	listener, err := createSocket(s.ctx, path, s.logger)
	if err != nil {
		return xerrors.Errorf("create socket: %w", err)
	}

	s.listener = listener
	s.path = path

	s.logger.Info(s.ctx, "agent socket server started", slog.F("path", path))

	// Start accepting connections
	s.wg.Add(1)
	go s.acceptConnections()

	return nil
}

// Stop stops the socket server
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener == nil {
		return nil
	}

	s.logger.Info(s.ctx, "stopping agent socket server")

	// Cancel context to stop accepting new connections
	s.cancel()

	// Close listener
	if err := s.listener.Close(); err != nil {
		s.logger.Warn(s.ctx, "error closing socket listener", slog.Error(err))
	}

	// Wait for all connections to finish
	s.wg.Wait()

	// Clean up socket file
	if err := cleanupSocket(s.path); err != nil {
		s.logger.Warn(s.ctx, "error cleaning up socket file", slog.Error(err))
	}

	s.listener = nil
	s.logger.Info(s.ctx, "agent socket server stopped")

	return nil
}

// RegisterHandler registers a handler for a method
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = handler
}

// GetPath returns the socket path
func (s *Server) GetPath() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

// acceptConnections accepts incoming connections
func (s *Server) acceptConnections() {
	defer s.wg.Done()

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

		// Handle connection in a goroutine
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConnection(conn)
		}()
	}
}

// handleConnection handles a single connection
func (s *Server) handleConnection(conn net.Conn) {
	defer conn.Close()

	// Authenticate connection first to get context
	ctx, err := s.authMiddleware.Authenticate(s.ctx, conn)
	if err != nil {
		s.logger.Warn(s.ctx, "authentication failed", slog.Error(err))
		return
	}

	// Set connection deadline
	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		s.logger.Warn(ctx, "failed to set connection deadline", slog.Error(err))
	}

	s.logger.Debug(ctx, "new connection accepted", slog.F("remote_addr", conn.RemoteAddr()))

	// Handle requests
	decoder := json.NewDecoder(conn)
	encoder := json.NewEncoder(conn)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Set read deadline
		if err := conn.SetReadDeadline(time.Now().Add(30 * time.Second)); err != nil {
			s.logger.Warn(ctx, "failed to set read deadline", slog.Error(err))
		}

		var req Request
		if err := decoder.Decode(&req); err != nil {
			if err == io.EOF {
				s.logger.Debug(ctx, "connection closed by client")
				return
			}
			s.logger.Warn(ctx, "error decoding request", slog.Error(err))

			// Send error response
			resp := NewErrorResponse("", NewError(ErrCodeParseError, "Parse error", err.Error()))
			_ = encoder.Encode(resp)
			return
		}

		// Handle request
		resp := s.handleRequest(ctx, &req)

		// Send response
		if err := encoder.Encode(resp); err != nil {
			s.logger.Warn(ctx, "error sending response", slog.Error(err))
			return
		}
	}
}

// handleRequest handles a single request
func (s *Server) handleRequest(ctx context.Context, req *Request) *Response {
	// Validate request
	if req.Version != ProtocolVersion {
		return NewErrorResponse(req.ID, NewError(ErrCodeInvalidRequest, "Unsupported version", req.Version))
	}

	if req.Method == "" {
		return NewErrorResponse(req.ID, NewError(ErrCodeInvalidRequest, "Missing method", nil))
	}

	// Get handler
	s.mu.RLock()
	handler, exists := s.handlers[req.Method]
	s.mu.RUnlock()

	if !exists {
		return NewErrorResponse(req.ID, NewError(ErrCodeMethodNotFound, "Method not found", req.Method))
	}

	// Call handler
	type requestIDKey struct{}
	ctx = context.WithValue(ctx, requestIDKey{}, req.ID)
	resp, err := handler(Context{}, req)
	if err != nil {
		s.logger.Warn(ctx, "handler execution failed", slog.Error(err))
		return NewErrorResponse(req.ID, NewError(ErrCodeInternalError, "Internal error", err.Error()))
	}

	return resp
}
