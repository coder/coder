package agent

import (
	"context"
	"net"
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
}

func newConnectionReporter(ctx context.Context, logger slog.Logger) *connectionReporter {
	return &connectionReporter{
		ctx:    ctx,
		logger: logger,
		report: make(chan *proto.ReportConnectionRequest, reportConnectionBufferLimit),
	}
}

// reportConnectionsLoop reports collected connection events to the API for
// auditing.
func (r *connectionReporter) reportLoop(ctx context.Context, sink connectionReportSink) error {
	for {
		select {
		case payload := <-r.report:
			logger := r.logger.With(slog.F("payload", payload))
			logger.Debug(ctx, "reporting connection")
			_, err := sink.ReportConnection(ctx, payload)
			if err != nil {
				// Do not fail the loop if we fail to report a connection, just
				// log a warning.
				// Related to https://github.com/coder/coder/issues/20194
				logger.Warn(ctx, "failed to report connection to server", slog.Error(err))
				// keep going, we still need to remove it from the slice
			} else {
				logger.Debug(ctx, "successfully reported connection")
			}
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

	payload := &proto.ReportConnectionRequest{
		Connection: &proto.Connection{
			Id:              connectEvent.ID[:],
			Action:          proto.Connection_CONNECT,
			Type:            connectEvent.Type,
			Timestamp:       timestamppb.New(time.Now()),
			Ip:              connectEvent.IP,
			StatusCode:      0,
			Reason:          nil,
			ClientSessionId: connectEvent.ClientSessionID,
		},
	}

	select {
	case r.report <- payload:
	default:
		r.logger.Warn(r.ctx, "connection report buffer limit reached, dropping report",
			slog.F("limit", reportConnectionBufferLimit),
			slog.F("payload", payload),
		)
	}

	return &disconnectionReporter{
		ctx:          r.ctx,
		connectEvent: &connectEvent,
		logger:       r.logger,
		report:       r.report,
	}
}

type disconnectionReporter struct {
	ctx          context.Context
	connectEvent *proto.ConnectEvent
	logger       slog.Logger
	report       chan *proto.ReportConnectionRequest
}

func (r *disconnectionReporter) Disconnect(disconnectEvent proto.DisconnectEvent) {
	payload := &proto.ReportConnectionRequest{
		Connection: &proto.Connection{
			Id:              r.connectEvent.ID[:],
			Action:          proto.Connection_DISCONNECT,
			Type:            r.connectEvent.Type,
			Timestamp:       timestamppb.New(time.Now()),
			Ip:              r.connectEvent.IP,
			StatusCode:      int32(disconnectEvent.Code), //nolint:gosec
			Reason:          &disconnectEvent.Reason,
			ClientSessionId: r.connectEvent.ClientSessionID,
		},
	}

	select {
	case r.report <- payload:
	default:
		r.logger.Warn(r.ctx, "connection report buffer limit reached, dropping report",
			slog.F("limit", reportConnectionBufferLimit),
			slog.F("payload", payload),
		)
	}
}
