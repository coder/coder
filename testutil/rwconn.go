package testutil

import (
	"io"
	"net"
	"time"
)

type ReaderWriterConn struct {
	io.Reader
	io.Writer
}

func (*ReaderWriterConn) Close() (err error) {
	return nil
}

func (*ReaderWriterConn) LocalAddr() net.Addr {
	return nil
}

func (*ReaderWriterConn) RemoteAddr() net.Addr {
	return nil
}

func (*ReaderWriterConn) SetDeadline(_ time.Time) error {
	return nil
}

func (*ReaderWriterConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (*ReaderWriterConn) SetWriteDeadline(_ time.Time) error {
	return nil
}
