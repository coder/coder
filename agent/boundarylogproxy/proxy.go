// Package boundarylogproxy provides a Unix socket server that receives boundary
// audit logs and forwards them to coderd via the agent API.
//
// Wire Format:
// Boundary sends length-prefixed protobuf messages over the Unix socket.
// Each message is:
//   - 4 bytes: big-endian uint32 length of the protobuf data
//   - N bytes: protobuf-encoded BoundaryLogsRequest
//
// Boundary must generate its proto types with the same field numbers as the
// agent proto (see agent/proto/agent.proto BoundaryLog and ReportBoundaryLogsRequest).
package boundarylogproxy

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"os"
	"sync"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

// Sender is the interface for sending boundary logs to coderd.
type Sender interface {
	ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error)
}

// Server listens on a Unix socket for boundary log messages and forwards them
// to coderd.
type Server struct {
	logger     slog.Logger
	socketPath string
	sender     func() Sender // Function to get sender (may be nil until agent connects)

	mu       sync.Mutex
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

// NewServer creates a new boundary log proxy server.
func NewServer(logger slog.Logger, socketPath string, sender func() Sender) *Server {
	return &Server{
		logger:     logger.Named("boundary-log-proxy"),
		socketPath: socketPath,
		sender:     sender,
	}
}

// Start begins listening for connections on the Unix socket.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.listener != nil {
		return xerrors.New("server already started")
	}

	// Remove existing socket file if it exists.
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return xerrors.Errorf("listen on socket: %w", err)
	}

	// Make socket world-writable so boundary process can connect.
	if err := os.Chmod(s.socketPath, 0o777); err != nil {
		_ = listener.Close()
		return xerrors.Errorf("chmod socket: %w", err)
	}

	s.listener = listener
	s.ctx, s.cancel = context.WithCancel(ctx)

	s.wg.Add(1)
	go s.acceptLoop()

	s.logger.Info(s.ctx, "boundary log proxy started", slog.F("socket_path", s.socketPath))
	return nil
}

// SocketPath returns the path to the Unix socket.
func (s *Server) SocketPath() string {
	return s.socketPath
}

func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if s.ctx.Err() != nil {
				return
			}
			s.logger.Warn(s.ctx, "accept error", slog.Error(err))
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(conn)
	}
}

func (s *Server) handleConnection(conn net.Conn) {
	defer s.wg.Done()
	defer conn.Close()

	for {
		select {
		case <-s.ctx.Done():
			return
		default:
		}

		// Read message length (4 bytes, big endian).
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Warn(s.ctx, "read length error", slog.Error(err))
			return
		}

		// Sanity check on length.
		if length > 1<<20 { // 1MB max
			s.logger.Warn(s.ctx, "message too large", slog.F("length", length))
			return
		}

		// Read message body.
		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
			s.logger.Warn(s.ctx, "read body error", slog.Error(err))
			return
		}

		// Unmarshal proto message. Boundary's proto wire format must match
		// the agent proto exactly (same field numbers and types).
		var req agentproto.ReportBoundaryLogsRequest
		if err := proto.Unmarshal(buf, &req); err != nil {
			s.logger.Warn(s.ctx, "unmarshal error", slog.Error(err))
			continue
		}

		// Forward to coderd.
		sender := s.sender()
		if sender == nil {
			s.logger.Debug(s.ctx, "dropping boundary logs, sender not available yet",
				slog.F("log_count", len(req.Logs)))
			continue
		}

		_, err := sender.ReportBoundaryLogs(s.ctx, &req)
		if err != nil {
			s.logger.Warn(s.ctx, "failed to forward boundary logs", slog.Error(err))
			continue
		}

		s.logger.Debug(s.ctx, "forwarded boundary logs", slog.F("log_count", len(req.Logs)))
	}
}

// Close stops the server and cleans up resources.
func (s *Server) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.cancel != nil {
		s.cancel()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.wg.Wait()

	// Clean up socket file.
	_ = os.Remove(s.socketPath)

	return nil
}
