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

	connectPath := connectionPath(&connectEvent)
	payload := connectionRequest(&connectEvent, connectPath, nil, proto.Connection_PATH_UNSPECIFIED)
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
		connectPath:  connectPath,
		logger:       r.logger,
		report:       r.report,
	}
}

type disconnectionReporter struct {
	ctx          context.Context
	connectEvent *proto.ConnectEvent
	connectPath  proto.Connection_Path
	logger       slog.Logger
	report       chan *proto.ReportConnectionRequest
}

func (r *disconnectionReporter) Disconnect(disconnectEvent proto.DisconnectEvent) {
	payload := connectionRequest(r.connectEvent, r.connectPath, &disconnectEvent, connectionPath(r.connectEvent))
	select {
	case r.report <- payload:
	default:
		r.logger.Warn(r.ctx, "connection report buffer limit reached, dropping report",
			slog.F("limit", reportConnectionBufferLimit),
			slog.F("payload", payload),
		)
	}
}

// connectionPath returns the current network path of the connection, if the
// connect event can report it.
func connectionPath(connect *proto.ConnectEvent) proto.Connection_Path {
	if connect.Path == nil {
		return proto.Connection_PATH_UNSPECIFIED
	}
	return connect.Path()
}

// connectionRequest converts connect and disconnect events into a proto
// connection request.  Disconnect events must pass both the original connect
// event and the disconnect event. The connect path is included in both, so the
// path is recorded even if only the disconnect report is delivered.
func connectionRequest(
	connect *proto.ConnectEvent, connectPath proto.Connection_Path,
	disconnect *proto.DisconnectEvent, disconnectPath proto.Connection_Path,
) *proto.ReportConnectionRequest {
	// TODO: For now, `connect.AppName` is ignored.  It needs to be added once the
	// type and app names are separated in the database.
	payload := &proto.ReportConnectionRequest{
		Connection: &proto.Connection{
			Id:              connect.ID[:],
			Action:          proto.Connection_CONNECT,
			Type:            connect.Type,
			Timestamp:       timestamppb.New(time.Now()),
			Ip:              connect.IP,
			StatusCode:      0,
			Reason:          nil,
			ClientSessionId: connect.ClientSessionID,
			ConnectPath:     connectPath,
		},
	}
	if disconnect != nil {
		payload.Connection.Action = proto.Connection_DISCONNECT
		payload.Connection.StatusCode = int32(disconnect.Code) //nolint:gosec
		payload.Connection.Reason = &disconnect.Reason
		payload.Connection.DisconnectPath = disconnectPath
	}
	return payload
}
