package agentssh

import (
	"context"
	"net"

	"github.com/gliderlabs/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/agent/proto"
	"github.com/coder/coder/v2/tailnet"
)

type connectionPathContextKey struct{}

func connectionPathFromContext(ctx ssh.Context) func() proto.Connection_Path {
	fn, _ := ctx.Value(connectionPathContextKey{}).(func() proto.Connection_Path)
	return fn
}

// ConnectionPathFunc returns a function that reports the current network path
// to the peer that opened conn, or nil if conn was not accepted from a tailnet
// listener.
func ConnectionPathFunc(conn net.Conn) func() proto.Connection_Path {
	pc, ok := tailnet.PeerConnFromConn(conn)
	if !ok {
		return nil
	}
	return func() proto.Connection_Path {
		return connectionPathToProto(pc.Path().Type)
	}
}

func connectionPathToProto(t tailnet.PathType) proto.Connection_Path {
	switch t {
	case tailnet.PathDERP:
		return proto.Connection_DERP
	case tailnet.PathDirect:
		return proto.Connection_DIRECT
	default:
		return proto.Connection_PATH_UNSPECIFIED
	}
}

// WatchConnectionPath logs the network path to the peer that opened conn, and
// logs again whenever the path changes between DERP and direct or the direct
// endpoint changes. The logger should carry the fields that identify the
// connection, such as the remote address and client session ID. Call the
// returned function to stop watching. It is a no-op if conn was not accepted
// from a tailnet listener.
func WatchConnectionPath(ctx context.Context, logger slog.Logger, conn net.Conn) (stop func()) {
	pc, ok := tailnet.PeerConnFromConn(conn)
	if !ok {
		return func() {}
	}
	if peerID, ok := pc.PeerID(); ok {
		logger = logger.With(slog.F("peer_id", peerID))
	}
	cur, cancel := pc.WatchPath(func(prev, cur tailnet.PeerPath) {
		logger.Info(ctx, "connection path changed",
			slog.F("previous_path", prev.Type),
			slog.F("previous_endpoint", prev.Endpoint),
			slog.F("path", cur.Type),
			slog.F("endpoint", cur.Endpoint),
			slog.F("derp_region", cur.DERPRegion),
		)
	})
	logger.Info(ctx, "initial connection path",
		slog.F("path", cur.Type),
		slog.F("endpoint", cur.Endpoint),
		slog.F("derp_region", cur.DERPRegion),
	)
	return cancel
}
