//nolint:revive,gocritic,errname,unconvert
package audit

import (
	"context"
	"log/slog"
	"net"
	"time"

	"golang.org/x/xerrors"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/coder/coder/v2/agent/boundarylogproxy/codec"
	agentproto "github.com/coder/coder/v2/agent/proto"
)

const (
	// The batch size and timer duration are chosen to provide reasonable responsiveness
	// for consumers of the aggregated logs while still minimizing the agent <-> coderd
	// network I/O when an AI agent is actively making network requests.
	defaultBatchSize          = 10
	defaultBatchTimerDuration = 5 * time.Second
)

// SocketAuditor implements the Auditor interface. It sends logs to the
// workspace agent's boundary log proxy socket. It queues logs and sends
// them in batches using a batch size and timer. The internal queue operates
// as a FIFO i.e., logs are sent in the order they are received and dropped
// if the queue is full.
type SocketAuditor struct {
	dial               func() (net.Conn, error)
	logger             *slog.Logger
	logCh              chan *agentproto.BoundaryLog
	batchSize          int
	batchTimerDuration time.Duration
	socketPath         string

	// onFlushAttempt is called after each flush attempt (intended for testing).
	onFlushAttempt func()
}

// NewSocketAuditor creates a new SocketAuditor that sends logs to the agent's
// boundary log proxy socket after SocketAuditor.Loop is called. The socket path
// is read from EnvAuditSocketPath, falling back to defaultAuditSocketPath.
func NewSocketAuditor(logger *slog.Logger, socketPath string) *SocketAuditor {
	// This channel buffer size intends to allow enough buffering for bursty
	// AI agent network requests while a batch is being sent to the workspace
	// agent.
	const logChBufSize = 2 * defaultBatchSize

	return &SocketAuditor{
		dial: func() (net.Conn, error) {
			return net.Dial("unix", socketPath)
		},
		logger:             logger,
		logCh:              make(chan *agentproto.BoundaryLog, logChBufSize),
		batchSize:          defaultBatchSize,
		batchTimerDuration: defaultBatchTimerDuration,
		socketPath:         socketPath,
	}
}

// AuditRequest implements the Auditor interface. It queues the log to be sent to the
// agent in a batch.
func (s *SocketAuditor) AuditRequest(req Request) {
	httpReq := &agentproto.BoundaryLog_HttpRequest{
		Method: req.Method,
		Url:    req.URL,
	}
	// Only include the matched rule for allowed requests. Boundary is deny by
	// default, so rules are what allow requests.
	if req.Allowed {
		httpReq.MatchedRule = req.Rule
	}

	log := &agentproto.BoundaryLog{
		Allowed:  req.Allowed,
		Time:     timestamppb.Now(),
		Resource: &agentproto.BoundaryLog_HttpRequest_{HttpRequest: httpReq},
	}

	select {
	case s.logCh <- log:
	default:
		s.logger.Warn("audit log dropped, channel full")
	}
}

// flushErr represents an error from flush, distinguishing between
// permanent errors (bad data) and transient errors (network issues).
type flushErr struct {
	err       error
	permanent bool
}

func (e *flushErr) Error() string { return e.err.Error() }

// flush sends the current batch of logs to the given connection.
func flush(conn net.Conn, logs []*agentproto.BoundaryLog) *flushErr {
	if len(logs) == 0 {
		return nil
	}

	req := &agentproto.ReportBoundaryLogsRequest{
		Logs: logs,
	}

	data, err := proto.Marshal(req)
	if err != nil {
		return &flushErr{err: err, permanent: true}
	}

	err = codec.WriteFrame(conn, codec.TagV1, data)
	if err != nil {
		return &flushErr{err: xerrors.Errorf("write frame: %x", err)}
	}
	return nil
}

// Loop handles the I/O to send audit logs to the agent.
func (s *SocketAuditor) Loop(ctx context.Context) {
	var conn net.Conn
	batch := make([]*agentproto.BoundaryLog, 0, s.batchSize)
	t := time.NewTimer(0)
	t.Stop()

	// connect attempts to establish a connection to the socket.
	connect := func() {
		if conn != nil {
			return
		}
		var err error
		conn, err = s.dial()
		if err != nil {
			s.logger.Warn("failed to connect to audit socket", "path", s.socketPath, "error", err)
			conn = nil
		}
	}

	// closeConn closes the current connection if open.
	closeConn := func() {
		if conn != nil {
			_ = conn.Close()
			conn = nil
		}
	}

	// clearBatch resets the length of the batch and frees memory while preserving
	// the batch slice backing array.
	clearBatch := func() {
		for i := range len(batch) {
			batch[i] = nil
		}
		batch = batch[:0]
	}

	// doFlush flushes the batch and handles errors by reconnecting.
	doFlush := func() {
		t.Stop()
		defer func() {
			if s.onFlushAttempt != nil {
				s.onFlushAttempt()
			}
		}()
		if len(batch) == 0 {
			return
		}
		connect()
		if conn == nil {
			// No connection: logs will be retried on next flush.
			s.logger.Warn("no connection to flush; resetting batch timer",
				"duration_sec", s.batchTimerDuration.Seconds(),
				"batch_size", len(batch))
			// Reset the timer so we aren't stuck waiting for the batch to fill
			// or a new log to arrive before the next attempt.
			t.Reset(s.batchTimerDuration)
			return
		}

		if err := flush(conn, batch); err != nil {
			if err.permanent {
				// Data error: discard batch to avoid infinite retries.
				s.logger.Warn("dropping batch due to data error on flush attempt",
					"error", err, "batch_size", len(batch))
				clearBatch()
			} else {
				// Network error: close connection but keep batch and retry.
				s.logger.Warn("failed to flush audit logs; resetting batch timer to reconnect and retry",
					"error", err, "duration_sec", s.batchTimerDuration.Seconds(),
					"batch_size", len(batch))
				closeConn()
				// Reset the timer so we aren't stuck waiting for a new log to
				// arrive before the next attempt.
				t.Reset(s.batchTimerDuration)
			}
			return
		}

		clearBatch()
	}

	connect()

	for {
		select {
		case <-ctx.Done():
			// Drain any pending logs before the last flush. Not concerned about
			// growing the batch slice here since we're exiting.
		drain:
			for {
				select {
				case log := <-s.logCh:
					batch = append(batch, log)
				default:
					break drain
				}
			}

			doFlush()
			closeConn()
			return
		case <-t.C:
			doFlush()
		case log := <-s.logCh:
			// If batch is at capacity, attempt flushing first and drop the log if
			// the batch still full.
			if len(batch) >= s.batchSize {
				doFlush()
				if len(batch) >= s.batchSize {
					s.logger.Warn("audit log dropped, batch full")
					continue
				}
			}

			batch = append(batch, log)

			if len(batch) == 1 {
				t.Reset(s.batchTimerDuration)
			}

			if len(batch) >= s.batchSize {
				doFlush()
			}
		}
	}
}
