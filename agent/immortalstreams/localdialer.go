package immortalstreams

import (
	"context"
	"net"
	"strconv"
	"strings"

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
//
// The LocalDialer can be created without a tailnet connection and will fall back
// to local dialing until UpdateTailnetConn is called.
type LocalDialer struct {
	logger slog.Logger

	// localDialer handles traditional local network connections
	localDialer *net.Dialer

	// tailnetConn provides access to the tailscale network (can be nil initially)
	tailnetConn *tailnet.Conn
}

// NewLocalDialer creates a new LocalDialer that can route connections
// intelligently between local and tailscale networks.
// The tailnetConn will be nil initially and updated later
// via UpdateTailnetConn.
func NewLocalDialer(logger slog.Logger) *LocalDialer {
	return &LocalDialer{
		logger:      logger.Named("local-dialer"),
		localDialer: &net.Dialer{},
		tailnetConn: nil,
	}
}

// UpdateTailnetConn updates the tailnet connection and agent address.
// This allows the LocalDialer to start using tailscale network routing.
func (d *LocalDialer) UpdateTailnetConn(tailnetConn *tailnet.Conn) {
	d.tailnetConn = tailnetConn
	d.logger.Debug(context.Background(), "updated tailnet connection")
}

// DialContext implements the Dialer interface that tries to connect to an
// in-process listener on the requested TCP port via tailnet.Conn.forwardTCP using net.Pipe.
// If no in-process listener is found, it falls back to dialing localhost.
// Only TCP connections are supported as immortal streams only need TCP.
func (d *LocalDialer) DialContext(ctx context.Context, address string) (net.Conn, error) {
	d.logger.Debug(ctx, "dialing connection",
		slog.F("address", address))

	// Parse the address and extract port for potential in-process listener dial
	// We ignore the host part of the address; we always dial localhost.
	_, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return nil, xerrors.Errorf("parse address %q: %w", address, err)
	}

	port, err := strconv.ParseUint(portStr, 10, 16)
	if err != nil {
		return nil, xerrors.Errorf("parse port %q: %w", portStr, err)
	}

	// If we have a tailnet connection, first attempt to connect to an
	// in-process listener on the requested TCP port via tailnet.Conn.forwardTCP using net.Pipe.
	if d.tailnetConn != nil {
		if c := d.tailnetConn.DialInternalTCP(ctx, uint16(port)); c != nil {
			d.logger.Debug(ctx, "connected to internal tailnet listener via pipe", slog.F("port", port))
			return c, nil
		}
	}

	return d.dialLocal(ctx, net.JoinHostPort("localhost", portStr))
}

// dialLocal uses the local network dialer
func (d *LocalDialer) dialLocal(ctx context.Context, address string) (net.Conn, error) {
	conn, err := d.localDialer.DialContext(ctx, "tcp", address)
	if err != nil {
		d.logger.Debug(ctx, "local dial failed", slog.Error(err))
		return nil, err
	}
	d.logger.Debug(ctx, "local dial succeeded")
	return conn, nil
}

// isConnectionRefusedError checks if an error indicates a connection was refused
// This is used to preserve the semantics of the original error handling
func isConnectionRefusedError(err error) bool {
	if err == nil {
		return false
	}

	// This uses the same logic as the original manager.go
	errStr := err.Error()
	return strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "connectex: No connection could be made because the target machine actively refused it") ||
		strings.Contains(errStr, "actively refused")
}
