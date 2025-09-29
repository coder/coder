package immortalstreams

import (
	"context"
	"net"
	"strconv"

	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/tailnet"
)

// LocalDialer is a Dialer implementation that intelligently chooses between
// local network connections and tailscale network connections based on the
// target address and context.
//
// It follows a similar approach to tailnet.forwardTCP and the netstack
// GetTCPHandlerForFlow pattern, providing context-aware routing for different
// connection types.
type LocalDialer struct {
	logger slog.Logger

	// tailnetConn provides access to the tailscale network (can be nil initially)
	tailnetConn *tailnet.Conn
}

// NewLocalDialer creates a new LocalDialer that can route connections
// intelligently between local and tailscale networks. If tailnetConn is nil,
// only local dialing will be attempted.
func NewLocalDialer(logger slog.Logger, tailnetConn *tailnet.Conn) *LocalDialer {
	return &LocalDialer{
		logger:      logger.Named("local-dialer"),
		tailnetConn: tailnetConn,
	}
}

// DialPort implements the Dialer interface that tries to connect to an
// in-process listener on the requested TCP port via tailnet.Conn.forwardTCP using net.Pipe.
// If no in-process listener is found, it falls back to dialing localhost.
// Only TCP connections are supported as immortal streams only need TCP.
func (d *LocalDialer) DialPort(ctx context.Context, port uint16) (net.Conn, error) {
	d.logger.Debug(ctx, "dialing connection",
		slog.F("port", port))

	// If we have a tailnet connection, first attempt to connect to an
	// in-process listener on the requested TCP port via tailnet.Conn.forwardTCP using net.Pipe.
	if d.tailnetConn != nil {
		if c, err := d.tailnetConn.DialInternalTCP(ctx, port); err == nil && c != nil {
			d.logger.Debug(ctx, "connected to internal tailnet listener via pipe", slog.F("port", port))
			return c, nil
		} else if err != nil {
			// Only fall back to local dial if there is no internal listener.
			if xerrors.Is(err, tailnet.ErrNoInternalListener) {
				d.logger.Debug(ctx, "no internal listener; falling back to local dial", slog.F("port", port))
			} else {
				d.logger.Debug(ctx, "internal tailnet dial failed", slog.Error(err), slog.F("port", port))
				return nil, err
			}
		}
	}

	return d.dialLocal(ctx, net.JoinHostPort("localhost", strconv.Itoa(int(port))))
}

// dialLocal uses the local network dialer
func (d *LocalDialer) dialLocal(ctx context.Context, address string) (net.Conn, error) {
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", address)
	if err != nil {
		d.logger.Debug(ctx, "local dial failed", slog.Error(err))
		return nil, err
	}
	d.logger.Debug(ctx, "local dial succeeded")
	return conn, nil
}
