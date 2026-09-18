package exitnode

import (
	"bufio"
	"bytes"
	"encoding/binary"
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
	// DefaultSniffTimeout bounds how long the exit node waits for the client
	// to send its first bytes before giving up on learning a host name.
	// Server-speaks-first protocols such as SSH never send anything until
	// the server does, so the timeout is the only way to move on for them.
	DefaultSniffTimeout = 2 * time.Second

	// sniffMaxBytes bounds the peek buffer. A TLS ClientHello with large
	// key shares or an HTTP request with many headers fits comfortably; if
	// the host has not been found by then, it is treated as unknown.
	sniffMaxBytes = 16 * 1024
)

// errNeedMore signals that the peeked prefix is not yet a complete message.
var errNeedMore = xerrors.New("need more bytes")

// SniffHost peeks at the first bytes the client sends and extracts a
// destination host from a TLS ClientHello SNI extension or an HTTP/1 Host
// header. It returns the host (empty when none was found) and a net.Conn that
// replays the peeked bytes before continuing with the original stream, so the
// application protocol is undisturbed.
//
// SniffHost never blocks past timeout. When the client is silent, the returned
// host is empty and the connection is returned unread. A non-timeout read
// error (for example the client hanging up) is returned as err together with
// a replaying conn for whatever was read.
func SniffHost(conn net.Conn, timeout time.Duration) (host string, replay net.Conn, err error) {
	buf := make([]byte, 0, 4096)
	replayConn := func() net.Conn {
		if len(buf) == 0 {
			return conn
		}
		return &replayingConn{Conn: conn, r: io.MultiReader(bytes.NewReader(buf), conn)}
	}

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return "", conn, xerrors.Errorf("set sniff deadline: %w", err)
	}
	defer func() {
		// Clearing the deadline must not mask a real read error. The
		// connection is unusable if this fails anyway, so the error is
		// dropped in favor of the primary result.
		_ = conn.SetReadDeadline(time.Time{})
	}()

	for len(buf) < sniffMaxBytes {
		if len(buf) == cap(buf) {
			grown := make([]byte, len(buf), min(cap(buf)*2, sniffMaxBytes))
			copy(grown, buf)
			buf = grown
		}
		n, readErr := conn.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]

		if len(buf) > 0 {
			host, parseErr := parseClientHost(buf)
			if parseErr == nil {
				return host, replayConn(), nil
			}
			if !errors.Is(parseErr, errNeedMore) {
				// Not a protocol we understand; stop reading immediately so
				// the application is not delayed.
				return "", replayConn(), nil
			}
		}

		if readErr != nil {
			if errors.Is(readErr, os.ErrDeadlineExceeded) {
				return "", replayConn(), nil
			}
			return "", replayConn(), readErr
		}
	}
	return "", replayConn(), nil
}

// parseClientHost inspects the start of a client stream. It returns
// errNeedMore when more bytes are required to decide, or another error when
// the bytes are not a protocol it understands.
func parseClientHost(buf []byte) (string, error) {
	if buf[0] == tlsRecordTypeHandshake {
		return parseTLSClientHelloSNI(buf)
	}
	if looksLikeHTTPRequest(buf) {
		return parseHTTPHost(buf)
	}
	return "", xerrors.New("unknown protocol")
}

var httpMethods = []string{
	http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
	http.MethodPatch, http.MethodDelete, http.MethodConnect,
	http.MethodOptions, http.MethodTrace,
}

// looksLikeHTTPRequest reports whether buf is, or is a prefix of, an HTTP/1
// request line for a known method.
func looksLikeHTTPRequest(buf []byte) bool {
	for _, m := range httpMethods {
		prefix := m + " "
		if len(buf) < len(prefix) {
			if strings.HasPrefix(prefix, string(buf)) {
				return true
			}
			continue
		}
		if string(buf[:len(prefix)]) == prefix {
			return true
		}
	}
	return false
}

func parseHTTPHost(buf []byte) (string, error) {
	if !bytes.Contains(buf, []byte("\r\n\r\n")) {
		return "", errNeedMore
	}
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(buf)))
	if err != nil {
		return "", xerrors.Errorf("read http request: %w", err)
	}
	return NormalizeHost(req.Host), nil
}

const (
	tlsRecordTypeHandshake   = 0x16
	tlsHandshakeClientHello  = 0x01
	tlsExtensionServerName   = 0x0000
	tlsServerNameTypeHost    = 0x00
	tlsRecordHeaderLen       = 5
	tlsHandshakeHeaderLen    = 4
	tlsClientHelloRandomLen  = 32
	tlsClientHelloVersionLen = 2
)

// parseTLSClientHelloSNI extracts the server_name from a TLS ClientHello. It
// only understands a ClientHello carried in a single TLS record, which covers
// every mainstream client; a multi-record hello yields an empty host.
func parseTLSClientHelloSNI(buf []byte) (string, error) {
	if len(buf) < tlsRecordHeaderLen {
		return "", errNeedMore
	}
	recordLen := int(binary.BigEndian.Uint16(buf[3:5]))
	if recordLen == 0 || recordLen > sniffMaxBytes-tlsRecordHeaderLen {
		return "", xerrors.New("tls record too large to sniff")
	}
	if len(buf) < tlsRecordHeaderLen+recordLen {
		return "", errNeedMore
	}
	hs := buf[tlsRecordHeaderLen : tlsRecordHeaderLen+recordLen]
	if len(hs) < tlsHandshakeHeaderLen || hs[0] != tlsHandshakeClientHello {
		return "", xerrors.New("not a client hello")
	}
	hsLen := int(hs[1])<<16 | int(hs[2])<<8 | int(hs[3])
	body := hs[tlsHandshakeHeaderLen:]
	if hsLen > len(body) {
		// The handshake spans multiple records, which the peek buffer does
		// not reassemble.
		return "", xerrors.New("client hello spans multiple records")
	}
	body = body[:hsLen]

	r := &byteReader{b: body}
	if !r.skip(tlsClientHelloVersionLen + tlsClientHelloRandomLen) {
		return "", xerrors.New("truncated client hello")
	}
	if !r.skipVec8() || !r.skipVec16() || !r.skipVec8() {
		return "", xerrors.New("truncated client hello")
	}
	if r.remaining() == 0 {
		// No extensions, so no SNI.
		return "", nil
	}
	exts, ok := r.vec16()
	if !ok {
		return "", xerrors.New("truncated extensions")
	}
	er := &byteReader{b: exts}
	for er.remaining() > 0 {
		extType, ok := er.uint16()
		if !ok {
			return "", xerrors.New("truncated extension")
		}
		extData, ok := er.vec16()
		if !ok {
			return "", xerrors.New("truncated extension")
		}
		if extType != tlsExtensionServerName {
			continue
		}
		nr := &byteReader{b: extData}
		list, ok := nr.vec16()
		if !ok {
			return "", xerrors.New("truncated server name list")
		}
		lr := &byteReader{b: list}
		for lr.remaining() > 0 {
			nameType, ok := lr.uint8()
			if !ok {
				return "", xerrors.New("truncated server name")
			}
			name, ok := lr.vec16()
			if !ok {
				return "", xerrors.New("truncated server name")
			}
			if nameType == tlsServerNameTypeHost {
				return NormalizeHost(string(name)), nil
			}
		}
		return "", nil
	}
	return "", nil
}

// byteReader is a tiny cursor over TLS vectors.
type byteReader struct {
	b   []byte
	off int
}

func (r *byteReader) remaining() int { return len(r.b) - r.off }

func (r *byteReader) skip(n int) bool {
	if r.remaining() < n {
		return false
	}
	r.off += n
	return true
}

func (r *byteReader) uint8() (uint8, bool) {
	if r.remaining() < 1 {
		return 0, false
	}
	v := r.b[r.off]
	r.off++
	return v, true
}

func (r *byteReader) uint16() (uint16, bool) {
	if r.remaining() < 2 {
		return 0, false
	}
	v := binary.BigEndian.Uint16(r.b[r.off:])
	r.off += 2
	return v, true
}

func (r *byteReader) vec8() ([]byte, bool) {
	n, ok := r.uint8()
	if !ok || r.remaining() < int(n) {
		return nil, false
	}
	v := r.b[r.off : r.off+int(n)]
	r.off += int(n)
	return v, true
}

func (r *byteReader) vec16() ([]byte, bool) {
	n, ok := r.uint16()
	if !ok || r.remaining() < int(n) {
		return nil, false
	}
	v := r.b[r.off : r.off+int(n)]
	r.off += int(n)
	return v, true
}

func (r *byteReader) skipVec8() bool {
	_, ok := r.vec8()
	return ok
}

func (r *byteReader) skipVec16() bool {
	_, ok := r.vec16()
	return ok
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
	return closeWrite(c.Conn)
}

// closeWriter is implemented by net.TCPConn, gonet.TCPConn, and the wrappers
// in this package.
type closeWriter interface {
	CloseWrite() error
}

// closeWrite half-closes conn for writing when supported and otherwise falls
// back to a full close, so the peer always observes EOF.
func closeWrite(conn net.Conn) error {
	if cw, ok := conn.(closeWriter); ok {
		return cw.CloseWrite()
	}
	return conn.Close()
}
