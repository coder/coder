package cliutil

import (
	"io"
	"net"
	"time"
)

type StdioConn struct {
	io.Reader
	io.Writer
}

func (*StdioConn) Close() (err error) {
	return nil
}

func (*StdioConn) LocalAddr() net.Addr {
	return nil
}

func (*StdioConn) RemoteAddr() net.Addr {
	return nil
}

func (*StdioConn) SetDeadline(_ time.Time) error {
	return nil
}

func (*StdioConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (*StdioConn) SetWriteDeadline(_ time.Time) error {
	return nil
}
