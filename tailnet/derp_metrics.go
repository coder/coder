package tailnet

import (
	"net"
	"sync/atomic"
)

// DERPWebsocketMetrics provides hooks for instrumenting DERP
// websocket connections. All methods must be safe for concurrent
// use.
type DERPWebsocketMetrics struct {
	// OnConnOpen is called when a DERP websocket connection is
	// accepted.
	OnConnOpen func()
	// OnConnClose is called when a DERP websocket connection is
	// closed.
	OnConnClose func()
	// OnRead is called after a successful Read with the number
	// of bytes read.
	OnRead func(n int)
	// OnWrite is called after a successful Write with the number
	// of bytes written.
	OnWrite func(n int)
}

// countingConn wraps a net.Conn and reports bytes read/written
// through the provided callbacks.
type countingConn struct {
	net.Conn
	metrics *DERPWebsocketMetrics
	closed  atomic.Bool
}

func newCountingConn(conn net.Conn, m *DERPWebsocketMetrics) *countingConn {
	return &countingConn{Conn: conn, metrics: m}
}

func (c *countingConn) Read(b []byte) (int, error) {
	n, err := c.Conn.Read(b)
	if n > 0 && c.metrics.OnRead != nil {
		c.metrics.OnRead(n)
	}
	return n, err
}

func (c *countingConn) Write(b []byte) (int, error) {
	n, err := c.Conn.Write(b)
	if n > 0 && c.metrics.OnWrite != nil {
		c.metrics.OnWrite(n)
	}
	return n, err
}

func (c *countingConn) Close() error {
	if c.closed.CompareAndSwap(false, true) {
		if c.metrics.OnConnClose != nil {
			c.metrics.OnConnClose()
		}
	}
	return c.Conn.Close()
}
