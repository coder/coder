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

// shutdownCause marks connections closed by the SSH server.
type shutdownCause struct{}

func (shutdownCause) fields() []slog.Field {
	return []slog.Field{
		codersdk.DisconnectReasonServerShutdown.SlogField(),
		codersdk.DisconnectReasonServerShutdown.SlogExpectedField(),
		codersdk.DisconnectInitiatorAgent.SlogField(),
	}
}

type shutdownContextKey struct{}

type trackedSSHConn struct {
	net.Conn
	clientSessionID string
	termination     atomic.Pointer[sshConnTermination]
}

// A nil shutdown cause records an earlier client or transport termination.
type sshConnTermination struct {
	shutdownCause *shutdownCause
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

func (c *trackedSSHConn) recordTermination(cause *shutdownCause) {
	// Transport cleanup can remain in the server's connection map after an
	// error or Close. Preserve that earlier cause before cleanup can block.
	c.termination.CompareAndSwap(nil, &sshConnTermination{shutdownCause: cause})
}

func (c *trackedSSHConn) shutdownCause() *shutdownCause {
	if termination := c.termination.Load(); termination != nil {
		return termination.shutdownCause
	}
	return nil
}

func connectionShutdownCause(ctx context.Context) *shutdownCause {
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

func (c *channelTermination) shutdownCause() *shutdownCause {
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

func sessionShutdownCause(ctx context.Context) *shutdownCause {
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
