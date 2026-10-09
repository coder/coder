package agentssh

import (
	"context"
	"errors"
	"net"
	"sync/atomic"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/codersdk"
)

// ShutdownCause describes why the server is closing its active connections.
type ShutdownCause struct {
	Reason      codersdk.DisconnectReason
	BuildID     string
	BuildReason string
	Transition  string
}

func (c ShutdownCause) fields() []slog.Field {
	initiator := codersdk.DisconnectInitiatorAgent
	if c.Reason == codersdk.DisconnectReasonWorkspaceStopped {
		initiator = codersdk.DisconnectInitiatorServer
	}
	fields := []slog.Field{c.Reason.SlogField(), c.Reason.SlogExpectedField(), initiator.SlogField()}
	if c.BuildID != "" {
		fields = append(fields, slog.F("build_id", c.BuildID), slog.F("build_reason", c.BuildReason), slog.F("transition", c.Transition))
	}
	return fields
}

type shutdownContextKey struct{}

type trackedSSHConn struct {
	net.Conn
	clientSessionID string
	termination     atomic.Pointer[sshConnTermination]
}

// A nil shutdown cause records an earlier client or transport termination.
type sshConnTermination struct {
	shutdownCause *ShutdownCause
}

func (c *trackedSSHConn) ClientSessionID() string { return c.clientSessionID }

func (c *trackedSSHConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	c.recordIOError(err)
	return n, err
}

func (c *trackedSSHConn) Write(p []byte) (int, error) {
	n, err := c.Conn.Write(p)
	c.recordIOError(err)
	return n, err
}

func (c *trackedSSHConn) Close() error {
	c.recordTermination(nil)
	return c.Conn.Close()
}

func (c *trackedSSHConn) recordIOError(err error) {
	if err == nil {
		return
	}
	if netErr, ok := errors.AsType[net.Error](err); ok && (netErr.Timeout() || netErr.Temporary()) {
		// A deadline or temporary failure does not itself close a transport.
		return
	}
	c.recordTermination(nil)
}

func (c *trackedSSHConn) recordTermination(cause *ShutdownCause) {
	// Transport cleanup can remain in the server's connection map after an
	// error or Close. Preserve that earlier cause before cleanup can block.
	c.termination.CompareAndSwap(nil, &sshConnTermination{shutdownCause: cause})
}

func (c *trackedSSHConn) shutdownCause() *ShutdownCause {
	if termination := c.termination.Load(); termination != nil {
		return termination.shutdownCause
	}
	return nil
}

func connectionShutdownCause(ctx context.Context) *ShutdownCause {
	if ctx == nil {
		return nil
	}
	if conn, ok := ctx.Value(shutdownContextKey{}).(*trackedSSHConn); ok {
		return conn.shutdownCause()
	}
	return nil
}

// Channel lifetime is independent of its SSH transport: a non-PTY command may
// keep running after the client closes its channel on a multiplexed connection.
type channelTermination struct {
	ctx         context.Context
	termination atomic.Pointer[sshConnTermination]
}

func (c *channelTermination) record() {
	c.termination.CompareAndSwap(nil, &sshConnTermination{shutdownCause: connectionShutdownCause(c.ctx)})
}

func (c *channelTermination) shutdownCause() *ShutdownCause {
	if termination := c.termination.Load(); termination != nil {
		return termination.shutdownCause
	}
	return connectionShutdownCause(c.ctx)
}

type channelTerminationContextKey struct{}

type channelContext struct {
	ssh.Context
	termination *channelTermination
}

func (c *channelContext) Value(key any) any {
	if _, ok := key.(channelTerminationContextKey); ok {
		return c.termination
	}
	return c.Context.Value(key)
}

func sessionChannelHandler(srv *ssh.Server, conn *gossh.ServerConn, newChan gossh.NewChannel, ctx ssh.Context) {
	termination := &channelTermination{ctx: ctx}
	defer termination.record()
	// DefaultSessionHandler returns when the channel's request stream closes,
	// independently of the process being served by sessionHandler.
	ssh.DefaultSessionHandler(srv, conn, newChan, &channelContext{Context: ctx, termination: termination})
}

func sessionShutdownCause(ctx context.Context) *ShutdownCause {
	if termination, ok := ctx.Value(channelTerminationContextKey{}).(*channelTermination); ok {
		return termination.shutdownCause()
	}
	return connectionShutdownCause(ctx)
}

func recordSessionTermination(ctx context.Context) {
	if termination, ok := ctx.Value(channelTerminationContextKey{}).(*channelTermination); ok {
		termination.record()
	}
}
