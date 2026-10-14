package agentssh

import (
	"context"
	"net"
	"sync/atomic"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// serverShutdownFields describes a connection closed by the SSH server.
func serverShutdownFields() []slog.Field {
	return []slog.Field{
		codersdk.DisconnectReasonServerShutdown.SlogField(),
		codersdk.DisconnectReasonServerShutdown.SlogExpectedField(),
		codersdk.DisconnectInitiatorAgent.SlogField(),
	}
}

type shutdownContextKey struct{}

// trackedSSHConn records whether Server.Close closed the connection. Handlers
// can finish after Close returns, so the flag lives on the connection rather
// than on the server.
type trackedSSHConn struct {
	net.Conn
	clientSessionID string
	shutdown        atomic.Bool
}

func (c *trackedSSHConn) ClientSessionID() string { return c.clientSessionID }

// closedByShutdown reports whether Server.Close closed the SSH connection that
// ctx belongs to.
func closedByShutdown(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	conn, ok := ctx.Value(shutdownContextKey{}).(*trackedSSHConn)
	return ok && conn.shutdown.Load()
}
