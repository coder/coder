package agent

import (
	"context"
	"net"
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
)

const (
	// reportConnectionBufferLimit limits the number of connection reports we
	// buffer to avoid growing the buffer indefinitely. This should not happen
	// unless the agent has lost connection to coderd for a long time or if
	// the agent is being spammed with connections.
	//
	// If we assume ~150 byte per connection report, this would be around 300KB
	// of memory which seems acceptable. We could reduce this if necessary by
	// not using the proto struct directly.
	reportConnectionBufferLimit = 2048
)

type connectionReportSink interface {
	ReportConnection(ctx context.Context, in *proto.ReportConnectionRequest) (*emptypb.Empty, error)
}

// connectionReporter is a subcomponent of the agent that handles connection and
// disconnection reports from the various connection listeners.
type connectionReporter struct {
	ctx    context.Context
	logger slog.Logger
	report chan *proto.ReportConnectionRequest

	mu   sync.Mutex
	cond *sync.Cond
	// open counts connections that have not reported their disconnect.
	open int
	// pending counts reports that are queued or being sent.
	pending int
}

func newConnectionReporter(ctx context.Context, logger slog.Logger) *connectionReporter {
	r := &connectionReporter{
		ctx:    ctx,
		logger: logger,
		report: make(chan *proto.ReportConnectionRequest, reportConnectionBufferLimit),
	}
	r.cond = sync.NewCond(&r.mu)
	return r
}

// enqueue queues a report for reportLoop, dropping it if the buffer is full.
func (r *connectionReporter) enqueue(payload *proto.ReportConnectionRequest) {
	r.mu.Lock()
	defer r.mu.Unlock()
	select {
	case r.report <- payload:
		r.pending++
	default:
		r.logger.Warn(r.ctx, "connection report buffer limit reached, dropping report",
			slog.F("limit", reportConnectionBufferLimit),
			slog.F("payload", payload),
		)
	}
}

// done marks a queued report as sent or failed.
func (r *connectionReporter) done() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pending--
	r.cond.Broadcast()
}

// closeConnection marks a connection as having reported its disconnect.
func (r *connectionReporter) closeConnection() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.open--
	r.cond.Broadcast()
}

// waitUntilReported blocks until every connection has reported its disconnect
// and every queued report has been sent or has failed, or until ctx ends.
// Shutdown uses it so the disconnects of the connections it closes reach
// coderd before the agent stops reporting.
func (r *connectionReporter) waitUntilReported(ctx context.Context) error {
	stop := context.AfterFunc(ctx, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		r.cond.Broadcast()
	})
	defer stop()
	r.mu.Lock()
	defer r.mu.Unlock()
	for (r.open > 0 || r.pending > 0) && ctx.Err() == nil {
		r.cond.Wait()
	}
	if r.open == 0 && r.pending == 0 {
		return nil
	}
	return ctx.Err()
}

// reportLoop reports collected connection events to the API for auditing.
func (r *connectionReporter) reportLoop(ctx context.Context, sink connectionReportSink) error {
	for {
		select {
		case payload := <-r.report:
			logger := r.logger.With(slog.F("payload", payload))
			logger.Debug(ctx, "reporting connection")
			_, err := sink.ReportConnection(ctx, payload)
			if err != nil {
				// Do not fail the loop if we fail to report a connection, just
				// log a warning and continue processing new reports.
				// Related to https://github.com/coder/coder/issues/20194
				logger.Warn(ctx, "failed to report connection to server", slog.Error(err))
			} else {
				logger.Debug(ctx, "successfully reported connection")
			}
			r.done()
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Connect reports a connection event, then returns a reporter that can report
// the disconnection event for that same connection.
func (r *connectionReporter) Connect(connectEvent proto.ConnectEvent) proto.DisconnectionReporter {
	// A blank IP can unfortunately happen if the connection is broken in a data
	// race before we get to introspect it. We still report it, and the recipient
	// can handle a blank IP.
	if connectEvent.IP != "" {
		// Remove the port from the IP because ports are not supported in coderd.
		if host, _, err := net.SplitHostPort(connectEvent.IP); err != nil {
			r.logger.Error(r.ctx, "split host and port for connection report failed",
				slog.F("ip", connectEvent.IP), slog.Error(err))
		} else {
			// Best effort.
			connectEvent.IP = host
		}
	}

	// If the IP is "localhost" (which it can be in some cases), set it to
	// 127.0.0.1 instead.
	// Related to https://github.com/coder/coder/issues/20194
	if connectEvent.IP == "localhost" {
		connectEvent.IP = "127.0.0.1"
	}

	r.mu.Lock()
	r.open++
	r.mu.Unlock()
	r.enqueue(connectionRequest(&connectEvent, nil))

	return &disconnectionReporter{
		reporter:     r,
		connectEvent: &connectEvent,
	}
}

type disconnectionReporter struct {
	reporter     *connectionReporter
	connectEvent *proto.ConnectEvent
	once         sync.Once
}

func (r *disconnectionReporter) Disconnect(disconnectEvent proto.DisconnectEvent) {
	r.once.Do(func() {
		// Queue the report before closing the connection, so a waiter never
		// sees both counts at zero while the report is still unsent.
		r.reporter.enqueue(connectionRequest(r.connectEvent, &disconnectEvent))
		r.reporter.closeConnection()
	})
}

// connectionRequest converts connect and disconnect events into a proto
// connection request.  Disconnect events must pass both the original connect
// event and the disconnect event.
func connectionRequest(connect *proto.ConnectEvent, disconnect *proto.DisconnectEvent) *proto.ReportConnectionRequest {
	payload := &proto.ReportConnectionRequest{
		Connection: &proto.Connection{
			Id:               connect.ID[:],
			Action:           proto.Connection_CONNECT,
			Timestamp:        timestamppb.New(time.Now()),
			Ip:               connect.IP,
			StatusCode:       0,
			Reason:           nil,
			ClientSessionId:  connect.ClientSessionID,
			AppName:          connect.AppName,
			ConnectionMethod: connect.Method,
		},
	}
	if disconnect != nil {
		payload.Connection.Action = proto.Connection_DISCONNECT
		payload.Connection.StatusCode = int32(disconnect.Code) //nolint:gosec
		payload.Connection.Reason = &disconnect.Reason
		payload.Connection.RxBytes = &disconnect.RxBytes
		payload.Connection.TxBytes = &disconnect.TxBytes
	}
	return payload
}
