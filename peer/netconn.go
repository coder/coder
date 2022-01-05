package peer

import (
	"net"
	"time"
)

type peerAddr struct{}

// Statically checks if we properly implement net.Addr.
var _ net.Addr = &peerAddr{}

func (a *peerAddr) Network() string {
	return "peer"
}

func (a *peerAddr) String() string {
	return "peer/unknown-addr"
}

type fakeNetConn struct {
	c    *Channel
	addr *peerAddr
}

// Statically checks if we properly implement net.Conn.
var _ net.Conn = &fakeNetConn{}

func (c *fakeNetConn) Read(b []byte) (n int, err error) {
	return c.c.Read(b)
}

func (c *fakeNetConn) Write(b []byte) (n int, err error) {
	return c.c.Write(b)
}

func (c *fakeNetConn) Close() error {
	return c.c.Close()
}

func (c *fakeNetConn) LocalAddr() net.Addr {
	return c.addr
}

func (c *fakeNetConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *fakeNetConn) SetDeadline(_ time.Time) error {
	return nil
}

func (c *fakeNetConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (c *fakeNetConn) SetWriteDeadline(_ time.Time) error {
	return nil
}
