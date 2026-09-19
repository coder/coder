// Package hostsniff learns a destination host name from the first bytes a
// client sends: the TLS SNI or the HTTP Host header. It is shared by the
// workspace agent and the exit node.
package hostsniff

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"
)

const (
	// DefaultTimeout bounds how long the exit node waits for the client
	// to send its first bytes before giving up on learning a host name.
	// Server-speaks-first protocols such as SSH never send anything until
	// the server does, so the timeout is the only way to move on for them.
	DefaultTimeout = 2 * time.Second

	// sniffMaxBytes bounds the peek buffer. A TLS ClientHello with large
	// key shares or an HTTP request with many headers fits comfortably; if
	// the host has not been found by then, it is treated as unknown.
	sniffMaxBytes = 16 * 1024

	tlsRecordTypeHandshake = 0x16
)

// errSniffDone aborts the TLS handshake once the ClientHello has been seen.
var errSniffDone = xerrors.New("sniff done")

// Host peeks at the first bytes the client sends and extracts a
// destination host from a TLS ClientHello SNI extension or an HTTP/1 Host
// header. It returns the host (empty when none was found) and a net.Conn that
// replays the peeked bytes before continuing with the original stream, so the
// application protocol is undisturbed.
//
// Host never blocks past timeout. When the client is silent, the returned
// host is empty and the connection is returned unread. A non-timeout read
// error (for example the client hanging up) is returned as err together with
// a replaying conn for whatever was read.
func Host(conn net.Conn, timeout time.Duration) (host string, replay net.Conn, err error) {
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", conn, xerrors.Errorf("set sniff deadline: %w", err)
	}
	// Clearing the deadline must not mask a real read error. The connection
	// is unusable if this fails anyway, so the error is dropped in favor of
	// the primary result.
	defer func() { _ = conn.SetReadDeadline(time.Time{}) }()

	// The first segment decides which parser runs; a protocol that is
	// neither TLS nor HTTP is handed on immediately so the application is
	// not delayed.
	sc := &sniffConn{Conn: conn}
	_, _ = sc.Read(make([]byte, 4096))
	sc.pos = 0
	switch {
	case len(sc.buf) == 0:
	case sc.buf[0] == tlsRecordTypeHandshake:
		var sni string
		_ = tls.Server(sc, &tls.Config{
			MinVersion: tls.VersionTLS12,
			GetConfigForClient: func(hi *tls.ClientHelloInfo) (*tls.Config, error) {
				sni = hi.ServerName
				return nil, errSniffDone
			},
		}).HandshakeContext(context.Background())
		host = NormalizeHost(sni)
	case looksLikeHTTPRequest(sc.buf):
		if req, err := http.ReadRequest(bufio.NewReader(sc)); err == nil {
			host = NormalizeHost(req.Host)
		}
	}

	replay = conn
	if len(sc.buf) > 0 {
		replay = &replayingConn{Conn: conn, r: io.MultiReader(bytes.NewReader(sc.buf), conn)}
	}
	if host == "" && sc.err != nil && !errors.Is(sc.err, os.ErrDeadlineExceeded) {
		return "", replay, sc.err
	}
	return host, replay, nil
}

// looksLikeHTTPRequest reports whether buf is, or is a prefix of, an HTTP/1
// request line for a known method.
func looksLikeHTTPRequest(buf []byte) bool {
	for _, m := range []string{"GET ", "HEAD ", "POST ", "PUT ", "PATCH ", "DELETE ", "CONNECT ", "OPTIONS ", "TRACE "} {
		if n := min(len(buf), len(m)); string(buf[:n]) == m[:n] {
			return true
		}
	}
	return false
}

// sniffConn feeds a parser the bytes read so far and then the live
// connection, recording everything (up to sniffMaxBytes) so it can be
// replayed. Writes are discarded so the TLS alerts crypto/tls emits when the
// handshake is aborted never reach the client.
type sniffConn struct {
	net.Conn
	buf []byte
	pos int
	// err is the first error the underlying connection returned.
	err error
}

func (c *sniffConn) Read(p []byte) (int, error) {
	if c.pos < len(c.buf) {
		n := copy(p, c.buf[c.pos:])
		c.pos += n
		return n, nil
	}
	if len(c.buf) >= sniffMaxBytes {
		return 0, io.EOF
	}
	n, err := c.Conn.Read(p[:min(len(p), sniffMaxBytes-len(c.buf))])
	c.buf = append(c.buf, p[:n]...)
	c.pos = len(c.buf)
	if err != nil && c.err == nil {
		c.err = err
	}
	return n, err
}

func (*sniffConn) Write(p []byte) (int, error) { return len(p), nil }

// Replay returns conn with early served ahead of any further reads, so bytes
// consumed while parsing a preamble are not lost to the application.
func Replay(conn net.Conn, early []byte) net.Conn {
	if len(early) == 0 {
		return conn
	}
	return &replayingConn{Conn: conn, r: io.MultiReader(bytes.NewReader(early), conn)}
}

// replayingConn serves reads from r, which replays previously peeked bytes
// before falling through to the underlying connection. All other methods,
// including half-close, are delegated to the wrapped conn.
type replayingConn struct {
	net.Conn
	r io.Reader
}

func (c *replayingConn) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

// CloseWrite half-closes the underlying connection when it supports it.
func (c *replayingConn) CloseWrite() error {
	return CloseWrite(c.Conn)
}

// CloseWrite half-closes conn for writing when supported (net.TCPConn,
// gonet.TCPConn, and the wrappers in this package) and otherwise falls back
// to a full close, so the peer always observes EOF.
func CloseWrite(conn net.Conn) error {
	if cw, ok := conn.(interface{ CloseWrite() error }); ok {
		return cw.CloseWrite()
	}
	return conn.Close()
}

// NormalizeHost lowercases a host name and strips a trailing dot and any
// port suffix so it can be compared against policy rules.
func NormalizeHost(host string) string {
	host = strings.TrimSpace(host)
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	} else if strings.HasPrefix(host, "[") && strings.HasSuffix(host, "]") {
		host = host[1 : len(host)-1]
	}
	return strings.ToLower(strings.TrimSuffix(host, "."))
}
