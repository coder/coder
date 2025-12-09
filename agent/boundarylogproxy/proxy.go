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

const (
	// logBufferSize is the size of the channel buffer for incoming logs.
	// If the buffer is full, new logs are dropped.
	logBufferSize = 1000
)

// Sender is the interface for sending boundary logs to coderd.
type Sender interface {
	ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error)
}

// Server listens on a Unix socket for boundary log messages and buffers them
// for forwarding to coderd. The socket server and the forwarder are decoupled:
// - Start() creates the socket and begins accepting connections
// - RunForwarder() drains the buffer and sends logs to coderd via the API
//
// This separation allows the socket to remain stable across API reconnections.
type Server struct {
	logger     slog.Logger
	socketPath string

	mu       sync.Mutex
	listener net.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// logs buffers incoming log requests for the forwarder to drain.
	logs chan *agentproto.ReportBoundaryLogsRequest
}

// NewServer creates a new boundary log proxy server.
func NewServer(logger slog.Logger, socketPath string) *Server {
	return &Server{
		logger:     logger.Named("boundary-log-proxy"),
		socketPath: socketPath,
		logs:       make(chan *agentproto.ReportBoundaryLogsRequest, logBufferSize),
	}
}

// Start begins listening for connections on the Unix socket.
// Incoming logs are buffered until RunForwarder drains them.
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

// RunForwarder drains the log buffer and forwards logs to coderd.
// This should be called via startAgentAPI to ensure the API client is always
// current and to handle reconnections properly. It blocks until ctx is canceled.
func (s *Server) RunForwarder(ctx context.Context, sender Sender) error {
	s.logger.Debug(ctx, "boundary log forwarder started")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req := <-s.logs:
			if _, err := sender.ReportBoundaryLogs(ctx, req); err != nil {
				s.logger.Warn(ctx, "failed to forward boundary logs",
					slog.Error(err),
					slog.F("log_count", len(req.Logs)))
				// Don't return the error - continue forwarding other logs.
				// The current batch is lost but the socket stays alive.
			} else {
				s.logger.Debug(ctx, "forwarded boundary logs", slog.F("log_count", len(req.Logs)))
			}
		}
	}
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

		// Unmarshal proto message.
		var req agentproto.ReportBoundaryLogsRequest
		if err := proto.Unmarshal(buf, &req); err != nil {
			s.logger.Warn(s.ctx, "unmarshal error", slog.Error(err))
			continue
		}

		// Buffer the logs for the forwarder. Non-blocking send - drop if full.
		select {
		case s.logs <- &req:
			// Buffered successfully.
		default:
			s.logger.Warn(s.ctx, "dropping boundary logs, buffer full",
				slog.F("log_count", len(req.Logs)))
		}
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
