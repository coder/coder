// Package boundarylogproxy provides a Unix socket server that receives boundary
// audit logs and forwards them to coderd via the agent API.
//
// Wire Format:
// Boundary sends tag and length prefixed protobuf messages over the Unix socket (TLV).
//   - 4 bits: big-endian tag (always 1 for now)
//   - 28 bits: big-endian length of the protobuf data
//   - length bytes: encoded protobuf data
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
	// logBufferSize is the size of the channel buffer for incoming log requests
	// from workspaces. This buffer size is intended to handle short bursts of workspaces
	// forwarding batches of logs in parallel.
	logBufferSize = 100
)

// Reporter reports boundary logs from workspaces.
type Reporter interface {
	ReportBoundaryLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) (*agentproto.ReportBoundaryLogsResponse, error)
}

// Server listens on a Unix socket for boundary log messages and buffers them
// for forwarding to coderd. The socket server and the forwarder are decoupled:
// - Start() creates the socket and accepts a connection from boundary
// - RunForwarder() drains the buffer and sends logs to coderd via AgentAPI
type Server struct {
	logger     slog.Logger
	socketPath string

	listener net.Listener
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

// Start begins listening for connections on the Unix socket, and handles new
// connections in a separate goroutine. Incoming logs from connections are
// buffered until RunForwarder drains them.
func (s *Server) Start() error {
	if err := os.Remove(s.socketPath); err != nil && !os.IsNotExist(err) {
		return xerrors.Errorf("remove existing socket: %w", err)
	}

	listener, err := net.Listen("unix", s.socketPath)
	if err != nil {
		return xerrors.Errorf("listen on socket: %w", err)
	}

	s.listener = listener
	ctx, cancel := context.WithCancel(context.Background())
	s.cancel = cancel

	s.wg.Add(1)
	go s.acceptLoop(ctx)

	s.logger.Info(ctx, "boundary log proxy started", slog.F("socket_path", s.socketPath))
	return nil
}

// RunForwarder drains the log buffer and forwards logs to coderd.
// It blocks until ctx is canceled.
func (s *Server) RunForwarder(ctx context.Context, sender Reporter) error {
	s.logger.Debug(ctx, "boundary log forwarder started")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case req := <-s.logs:
			_, err := sender.ReportBoundaryLogs(ctx, req)
			if err != nil {
				s.logger.Warn(ctx, "failed to forward boundary logs",
					slog.Error(err),
					slog.F("log_count", len(req.Logs)))
				// Continue forwarding other logs. The current batch is lost,
				// but the socket stays alive.
			}
		}
	}
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.logger.Warn(ctx, "socket accept error", slog.Error(err))
			continue
		}

		s.wg.Add(1)
		go s.handleConnection(ctx, conn)
	}
}

func (s *Server) handleConnection(ctx context.Context, conn net.Conn) {
	defer s.wg.Done()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		<-ctx.Done()
		_ = conn.Close()
	}()

	// Even though the length of data received can be larger than maxMsgSize,
	// practically they are not expected to be. This is a sanity check and
	// allows re-using a small fixed size read buffer.
	const maxMsgSize = 1 << 15
	buf := make([]byte, maxMsgSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var header uint32
		if err := binary.Read(conn, binary.BigEndian, &header); err != nil {
			if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
				return
			}
			s.logger.Warn(ctx, "read length error", slog.Error(err))
			return
		}

		length := header & 0x0FFFFFFF
		tag := header >> 28

		if tag != 1 {
			s.logger.Warn(ctx, "invalid tag value", slog.F("tag", tag))
			return
		}

		if length > maxMsgSize {
			s.logger.Warn(ctx, "message too large", slog.F("length", length))
			return
		}

		if _, err := io.ReadFull(conn, buf[:length]); err != nil {
			s.logger.Warn(ctx, "read full request error", slog.Error(err))
			return
		}

		var req agentproto.ReportBoundaryLogsRequest
		if err := proto.Unmarshal(buf[:length], &req); err != nil {
			s.logger.Warn(ctx, "proto unmarshal error", slog.Error(err))
			continue
		}

		select {
		case s.logs <- &req:
		default:
			s.logger.Warn(ctx, "dropping boundary logs, buffer full",
				slog.F("log_count", len(req.Logs)))
		}
	}
}

// Close stops the server and blocks until resources have been cleaned up.
// It must be called after Start.
func (s *Server) Close() error {
	if s.cancel != nil {
		s.cancel()
	}

	if s.listener != nil {
		_ = s.listener.Close()
	}

	s.wg.Wait()

	err := os.Remove(s.socketPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}
