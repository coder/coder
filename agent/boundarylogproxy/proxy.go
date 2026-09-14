// Package boundarylogproxy provides a Unix socket server that receives boundary
// audit logs and forwards them to coderd via the agent API.
package boundarylogproxy

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

const (
	// logBufferSize is the size of the channel buffer for incoming log requests
	// from workspaces. This buffer size is intended to handle short bursts of workspaces
	// forwarding batches of logs in parallel.
	logBufferSize = 100
)

const (
	droppedReasonBoundaryChannelFull = "boundary_channel_full"
	droppedReasonBoundaryBatchFull   = "boundary_batch_full"
	droppedReasonBufferFull          = "buffer_full"
	droppedReasonForwardFailed       = "forward_failed"
)

// DefaultSocketPath returns the default path for the boundary audit log socket.
func DefaultSocketPath() string {
	return filepath.Join(os.TempDir(), "boundary-audit.sock")
}

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
	metrics    *Metrics

	listener net.Listener
	cancel   context.CancelFunc
	wg       sync.WaitGroup

	// logs buffers incoming log requests for the forwarder to drain.
	logs chan *agentproto.ReportBoundaryLogsRequest
}

// NewServer creates a new boundary log proxy server.
func NewServer(logger slog.Logger, socketPath string, registerer prometheus.Registerer) *Server {
	return &Server{
		logger:     logger.Named("boundary-log-proxy"),
		socketPath: socketPath,
		metrics:    newMetrics(registerer),
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
				s.metrics.batchesDropped.WithLabelValues(droppedReasonForwardFailed).Inc()
				s.metrics.logsDropped.WithLabelValues(droppedReasonForwardFailed).Add(float64(len(req.Logs)))
				// Continue forwarding other logs. The current batch is lost,
				// but the socket stays alive.
				continue
			}
			s.metrics.batchesForwarded.Inc()
		}
	}
}

func (s *Server) acceptLoop(ctx context.Context) {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				s.logger.Warn(ctx, "accept loop terminated", slog.Error(ctx.Err()))
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

	// This is intended to be a sane starting point for the read buffer size.
	// It may be grown by codec.ReadMessage if necessary.
	const initBufSize = 1 << 10
	buf := make([]byte, initBufSize)

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		var err error
		var msg proto.Message
		msg, buf, err = codec.ReadMessage(conn, buf)
		switch {
		case errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed):
			return
		case errors.Is(err, codec.ErrUnsupportedTag) || errors.Is(err, codec.ErrMessageTooLarge):
			s.logger.Warn(ctx, "read frame error", slog.Error(err))
			return
		case err != nil:
			s.logger.Warn(ctx, "read message error", slog.Error(err))
			continue
		}

		s.handleMessage(ctx, msg)
	}
}

func (s *Server) handleMessage(ctx context.Context, msg proto.Message) {
	switch m := msg.(type) {
	case *agentproto.ReportBoundaryLogsRequest:
		s.bufferLogs(ctx, m)
	case *codec.BoundaryMessage:
		switch inner := m.Msg.(type) {
		case *codec.BoundaryMessage_Logs:
			s.bufferLogs(ctx, inner.Logs)
		case *codec.BoundaryMessage_Status:
			s.recordBoundaryStatus(inner.Status)
		default:
			s.logger.Warn(ctx, "unknown BoundaryMessage variant")
		}
	default:
		s.logger.Warn(ctx, "unexpected message type")
	}
}

func (s *Server) recordBoundaryStatus(status *codec.BoundaryStatus) {
	if n := status.DroppedChannelFull; n > 0 {
		s.metrics.logsDropped.WithLabelValues(droppedReasonBoundaryChannelFull).Add(float64(n))
	}
	if n := status.DroppedBatchFull; n > 0 {
		s.metrics.logsDropped.WithLabelValues(droppedReasonBoundaryBatchFull).Add(float64(n))
	}
}

func (s *Server) bufferLogs(ctx context.Context, req *agentproto.ReportBoundaryLogsRequest) {
	select {
	case s.logs <- req:
	default:
		s.logger.Warn(ctx, "dropping boundary logs, buffer full",
			slog.F("log_count", len(req.Logs)))
		s.metrics.batchesDropped.WithLabelValues(droppedReasonBufferFull).Inc()
		s.metrics.logsDropped.WithLabelValues(droppedReasonBufferFull).Add(float64(len(req.Logs)))
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
