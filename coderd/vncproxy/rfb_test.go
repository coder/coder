//nolint:testpackage // Exercises unexported parser helpers like s2cServerCutText.
package vncproxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"net"
	"sync"
	"testing"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/testutil"
)

// fakeConn wraps a Reader/Writer pair into an io.ReadWriteCloser. The
// parser's BicopyDropClipboard expects ReadWriteCloser; tests pump
// known byte sequences in and out.
type fakeConn struct {
	io.Reader
	io.Writer

	closeOnce sync.Once
	closed    chan struct{}
}

func newFakeConn(r io.Reader, w io.Writer) *fakeConn {
	return &fakeConn{
		Reader: r,
		Writer: w,
		closed: make(chan struct{}),
	}
}

func (f *fakeConn) Close() error {
	f.closeOnce.Do(func() { close(f.closed) })
	return nil
}

// rfb38ServerVersion is the 12-byte ProtocolVersion string a server
// running RFB 3.8 sends. Exposed as a constant so the encoded test
// fixtures match what the parser expects byte-for-byte.
var (
	rfb38ServerVersion = []byte("RFB 003.008\n")
	rfb38ClientVersion = []byte("RFB 003.008\n")
)

// encodeServerInit assembles a ServerInit message: 2 width + 2 height +
// 16 PIXEL_FORMAT (with bpp=32) + 4 name-length + N name. Tests use it
// to drive the handshake forward.
func encodeServerInit(name string, bpp byte) []byte {
	buf := make([]byte, 24+len(name))
	binary.BigEndian.PutUint16(buf[0:2], 800)   // width
	binary.BigEndian.PutUint16(buf[2:4], 600)   // height
	buf[4] = bpp                                // bits-per-pixel
	buf[5] = bpp                                // depth
	buf[6] = 0                                  // big-endian-flag
	buf[7] = 1                                  // true-color-flag
	binary.BigEndian.PutUint16(buf[8:10], 255)  // red-max
	binary.BigEndian.PutUint16(buf[10:12], 255) // green-max
	binary.BigEndian.PutUint16(buf[12:14], 255) // blue-max
	buf[14] = 16                                // red-shift
	buf[15] = 8                                 // green-shift
	buf[16] = 0                                 // blue-shift
	// 17..19 padding
	binary.BigEndian.PutUint32(buf[20:24], uint32(len(name))) //nolint:gosec // bounded by RFB protocol field widths
	copy(buf[24:], name)
	return buf
}

// rfb38NoneHandshakeServerBytes is the byte sequence a real RFB 3.8
// server with SecurityTypes=None speaks during the handshake, in
// order: ProtocolVersion, security-types list with None, security
// result OK, ServerInit. Tests pipe this into the parser and assert
// the same bytes come out the client side.
func rfb38NoneHandshakeServerBytes(serverInit []byte) []byte {
	var buf bytes.Buffer
	_, _ = buf.Write(rfb38ServerVersion)
	// Security types: count=1, list=[1] (None).
	_ = buf.WriteByte(1)
	_ = buf.WriteByte(1)
	// SecurityResult: 0 (OK).
	_ = binary.Write(&buf, binary.BigEndian, uint32(0))
	_, _ = buf.Write(serverInit)
	return buf.Bytes()
}

// rfb38NoneHandshakeClientBytes is the byte sequence a real RFB 3.8
// client with SecurityTypes=None sends during the handshake.
func rfb38NoneHandshakeClientBytes(sharedFlag byte) []byte {
	var buf bytes.Buffer
	_, _ = buf.Write(rfb38ClientVersion)
	// Client security choice: 1 (None).
	_ = buf.WriteByte(1)
	// ClientInit: shared-flag.
	_ = buf.WriteByte(sharedFlag)
	return buf.Bytes()
}

// runParser drives BicopyDropClipboard end-to-end with the given
// scripted server-to-parser bytes and client-to-parser bytes. It
// returns whatever bytes the parser forwarded to each side and any
// drop events observed.
//
// Setup: clientWire and serverWire are net.Pipe halves, so the
// fakeConn wired to the parser as "client" pumps bytes from the test's
// client script in and captures bytes the parser writes back. Same for
// server.
func runParser(t *testing.T, ctx context.Context, clientToParser, serverToParser []byte) (parserToClient, parserToServer []byte, drops []DropEvent, err error) { //nolint:revive // testing.T must come first by convention; ctx is the second logical input.
	t.Helper()

	// The parser has client and server "sides". From the parser's
	// perspective, the client connection is read for C2S bytes and
	// written for S2C bytes; vice versa for server.
	clientReader := bytes.NewReader(clientToParser)
	serverReader := bytes.NewReader(serverToParser)
	var clientWritten bytes.Buffer
	var serverWritten bytes.Buffer

	clientConn := newFakeConn(clientReader, &clientWritten)
	serverConn := newFakeConn(serverReader, &serverWritten)

	var dropMu sync.Mutex
	var got []DropEvent

	done := make(chan error, 1)
	go func() {
		done <- BicopyDropClipboard(ctx, clientConn, serverConn, func(ev DropEvent) {
			dropMu.Lock()
			got = append(got, ev)
			dropMu.Unlock()
		})
	}()

	select {
	case e := <-done:
		err = e
	case <-time.After(testutil.WaitShort):
		_ = clientConn.Close()
		_ = serverConn.Close()
		<-done
		err = xerrors.New("BicopyDropClipboard timed out")
	}
	return clientWritten.Bytes(), serverWritten.Bytes(), got, err
}

func TestHandshakePassthroughRFB38None(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	serverInit := encodeServerInit("desktop", 32)
	serverBytes := rfb38NoneHandshakeServerBytes(serverInit)
	// Client side speaks its handshake then closes; no messages.
	clientBytes := rfb38NoneHandshakeClientBytes(1)

	toClient, toServer, drops, err := runParser(t, ctx, clientBytes, serverBytes)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 0 {
		t.Fatalf("expected no drops during handshake, got %v", drops)
	}
	// The parser should forward the entire server handshake to the
	// client, byte-for-byte.
	if !bytes.Equal(toClient, serverBytes) {
		t.Fatalf("client received different handshake bytes\nwant: %x\ngot:  %x", serverBytes, toClient)
	}
	// And the entire client handshake to the server.
	if !bytes.Equal(toServer, clientBytes) {
		t.Fatalf("server received different handshake bytes\nwant: %x\ngot:  %x", clientBytes, toServer)
	}
}

func TestClientCutTextDropped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Build a client message stream: handshake, then ClientCutText, then
	// a KeyEvent the parser must forward unmodified.
	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	// ClientCutText with a 12-byte payload.
	cutText := []byte("hello world!")
	_ = clientStream.WriteByte(c2sClientCutText)
	_, _ = clientStream.Write([]byte{0, 0, 0})                              // padding
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(len(cutText))) //nolint:gosec // bounded by RFB protocol field widths
	_, _ = clientStream.Write(cutText)
	// KeyEvent (down=1, key=0x41).
	_ = clientStream.WriteByte(c2sKeyEvent)
	_ = clientStream.WriteByte(1)
	_, _ = clientStream.Write([]byte{0, 0})
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(0x41))

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, drops, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 {
		t.Fatalf("expected 1 drop, got %d: %v", len(drops), drops)
	}
	if drops[0].Direction != DirClientToServer {
		t.Errorf("drop direction = %v, want DirClientToServer", drops[0].Direction)
	}
	if drops[0].MessageType != c2sClientCutText {
		t.Errorf("drop message type = %d, want %d", drops[0].MessageType, c2sClientCutText)
	}
	if drops[0].PayloadLen != len(cutText) {
		t.Errorf("drop payload len = %d, want %d", drops[0].PayloadLen, len(cutText))
	}
	// What the server received: the client handshake, followed
	// immediately by the KeyEvent (no CutText in between).
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeClientBytes(1))
	_ = want.WriteByte(c2sKeyEvent)
	_ = want.WriteByte(1)
	_, _ = want.Write([]byte{0, 0})
	_ = binary.Write(&want, binary.BigEndian, uint32(0x41))
	if !bytes.Equal(toServer, want.Bytes()) {
		t.Fatalf("server bytes differ from expected\nwant: %x\ngot:  %x", want.Bytes(), toServer)
	}
}

func TestServerCutTextDropped(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var serverStream bytes.Buffer
	serverInit := encodeServerInit("d", 32)
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	// ServerCutText with a 5-byte payload.
	_ = serverStream.WriteByte(s2cServerCutText)
	_, _ = serverStream.Write([]byte{0, 0, 0})
	_ = binary.Write(&serverStream, binary.BigEndian, uint32(5))
	_, _ = serverStream.WriteString("12345")
	// Bell (1 byte, no payload) the parser must forward.
	_ = serverStream.WriteByte(s2cBell)

	clientStream := rfb38NoneHandshakeClientBytes(1)

	toClient, _, drops, err := runParser(t, ctx, clientStream, serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 {
		t.Fatalf("expected 1 drop, got %d: %v", len(drops), drops)
	}
	if drops[0].Direction != DirServerToClient {
		t.Errorf("drop direction = %v, want DirServerToClient", drops[0].Direction)
	}
	if drops[0].PayloadLen != 5 {
		t.Errorf("drop payload len = %d, want 5", drops[0].PayloadLen)
	}
	// Client received: server handshake + Bell (no CutText).
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_ = want.WriteByte(s2cBell)
	if !bytes.Equal(toClient, want.Bytes()) {
		t.Fatalf("client bytes differ\nwant: %x\ngot:  %x", want.Bytes(), toClient)
	}
}

func TestClientCutTextZeroLength(t *testing.T) {
	// A zero-length CutText is legal per RFC 6143 and must still be
	// dropped without forwarding the 8-byte header.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sClientCutText)
	_, _ = clientStream.Write([]byte{0, 0, 0})
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(0))

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, drops, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 {
		t.Fatalf("expected 1 drop, got %d", len(drops))
	}
	if drops[0].PayloadLen != 0 {
		t.Errorf("drop payload len = %d, want 0", drops[0].PayloadLen)
	}
	// Server saw only the handshake.
	if !bytes.Equal(toServer, rfb38NoneHandshakeClientBytes(1)) {
		t.Fatalf("server should have received only handshake, got %d extra bytes", len(toServer)-12-1-1)
	}
}

func TestClientCutTextLargePayload(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	const payloadSize = 1024
	payload := bytes.Repeat([]byte{'A'}, payloadSize)

	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sClientCutText)
	_, _ = clientStream.Write([]byte{0, 0, 0})
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(payloadSize))
	_, _ = clientStream.Write(payload)

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, drops, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 {
		t.Fatalf("expected 1 drop, got %d", len(drops))
	}
	if drops[0].PayloadLen != payloadSize {
		t.Errorf("drop payload len = %d, want %d", drops[0].PayloadLen, payloadSize)
	}
	if !bytes.Equal(toServer, rfb38NoneHandshakeClientBytes(1)) {
		t.Fatalf("server got unexpected bytes after handshake: %x", toServer[len(rfb38NoneHandshakeClientBytes(1)):])
	}
}

func TestSetEncodingsRewrite(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Client sends SetEncodings with [Tight=7, Hextile, CopyRect, ZRLE=16,
	// Cursor]. The parser must rewrite this to [Hextile, CopyRect,
	// Cursor] (3 entries) and update the count field accordingly.
	originalEncodings := []int32{
		7, // Tight
		encodingHextile,
		encodingCopyRect,
		16, // ZRLE
		encodingCursor,
	}
	expectedEncodings := []int32{
		encodingHextile,
		encodingCopyRect,
		encodingCursor,
	}

	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sSetEncodings)
	_ = clientStream.WriteByte(0)                                                     // padding
	_ = binary.Write(&clientStream, binary.BigEndian, uint16(len(originalEncodings))) //nolint:gosec // bounded by RFB protocol field widths
	for _, e := range originalEncodings {
		_ = binary.Write(&clientStream, binary.BigEndian, uint32(e)) //nolint:gosec // bounded by RFB protocol field widths
	}

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, drops, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 0 {
		t.Fatalf("SetEncodings rewrite should not produce drop events, got %v", drops)
	}
	// What the server received: handshake + rewritten SetEncodings.
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeClientBytes(1))
	_ = want.WriteByte(c2sSetEncodings)
	_ = want.WriteByte(0)
	_ = binary.Write(&want, binary.BigEndian, uint16(len(expectedEncodings))) //nolint:gosec // bounded by RFB protocol field widths
	for _, e := range expectedEncodings {
		_ = binary.Write(&want, binary.BigEndian, uint32(e)) //nolint:gosec // bounded by RFB protocol field widths
	}
	if !bytes.Equal(toServer, want.Bytes()) {
		t.Fatalf("server bytes differ from expected rewrite\nwant: %x\ngot:  %x", want.Bytes(), toServer)
	}
}

func TestSetEncodingsPassthroughWhenAllAllowed(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	encs := []int32{encodingHextile, encodingRaw, encodingCopyRect, encodingDesktopSize}
	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sSetEncodings)
	_ = clientStream.WriteByte(0)
	_ = binary.Write(&clientStream, binary.BigEndian, uint16(len(encs))) //nolint:gosec // bounded by RFB protocol field widths
	for _, e := range encs {
		_ = binary.Write(&clientStream, binary.BigEndian, uint32(e)) //nolint:gosec // bounded by RFB protocol field widths
	}

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, _, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if !bytes.Equal(toServer, clientStream.Bytes()) {
		t.Fatalf("expected SetEncodings to pass through unchanged when all encodings allowed")
	}
}

func TestSetEncodingsAllStripped(t *testing.T) {
	// If every encoding is on the deny list the parser must still
	// forward a valid SetEncodings (count=0), so the server uses Raw.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	encs := []int32{7, 16, -260} // Tight, ZRLE, TightPNG
	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sSetEncodings)
	_ = clientStream.WriteByte(0)
	_ = binary.Write(&clientStream, binary.BigEndian, uint16(len(encs))) //nolint:gosec // bounded by RFB protocol field widths
	for _, e := range encs {
		_ = binary.Write(&clientStream, binary.BigEndian, uint32(e)) //nolint:gosec // bounded by RFB protocol field widths
	}

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	_, toServer, _, err := runParser(t, ctx, clientStream.Bytes(), serverStream)
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeClientBytes(1))
	_ = want.WriteByte(c2sSetEncodings)
	_ = want.WriteByte(0)
	_ = binary.Write(&want, binary.BigEndian, uint16(0))
	if !bytes.Equal(toServer, want.Bytes()) {
		t.Fatalf("expected count=0 SetEncodings\nwant: %x\ngot:  %x", want.Bytes(), toServer)
	}
}

func TestFramebufferUpdateRawForwarded(t *testing.T) {
	// Hand the parser a FramebufferUpdate containing a single Raw
	// rectangle of 2x2 pixels at 32 bpp (16-byte body) and verify the
	// bytes round-trip exactly.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	rectBody := []byte{
		0x10, 0x20, 0x30, 0x40,
		0x11, 0x21, 0x31, 0x41,
		0x12, 0x22, 0x32, 0x42,
		0x13, 0x23, 0x33, 0x43,
	}
	var update bytes.Buffer
	_ = update.WriteByte(s2cFramebufferUpdate)
	_ = update.WriteByte(0)                                // padding
	_ = binary.Write(&update, binary.BigEndian, uint16(1)) // 1 rect
	// Rectangle header.
	_ = binary.Write(&update, binary.BigEndian, uint16(0))           // x
	_ = binary.Write(&update, binary.BigEndian, uint16(0))           // y
	_ = binary.Write(&update, binary.BigEndian, uint16(2))           // w
	_ = binary.Write(&update, binary.BigEndian, uint16(2))           // h
	_ = binary.Write(&update, binary.BigEndian, uint32(encodingRaw)) // encoding
	_, _ = update.Write(rectBody)

	var serverStream bytes.Buffer
	serverInit := encodeServerInit("d", 32)
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update.Bytes())

	clientStream := rfb38NoneHandshakeClientBytes(1)

	toClient, _, drops, err := runParser(t, ctx, clientStream, serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 0 {
		t.Fatalf("expected no drops, got %v", drops)
	}
	if !bytes.Equal(toClient, serverStream.Bytes()) {
		t.Fatalf("Raw FramebufferUpdate not forwarded byte-exact\nwant: %x\ngot:  %x", serverStream.Bytes(), toClient)
	}
}

func TestFramebufferUpdateCopyRectForwarded(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var update bytes.Buffer
	_ = update.WriteByte(s2cFramebufferUpdate)
	_ = update.WriteByte(0)
	_ = binary.Write(&update, binary.BigEndian, uint16(1))
	_ = binary.Write(&update, binary.BigEndian, uint16(10))
	_ = binary.Write(&update, binary.BigEndian, uint16(20))
	_ = binary.Write(&update, binary.BigEndian, uint16(50))
	_ = binary.Write(&update, binary.BigEndian, uint16(60))
	_ = binary.Write(&update, binary.BigEndian, uint32(encodingCopyRect))
	// CopyRect body: 2 src-x + 2 src-y.
	_ = binary.Write(&update, binary.BigEndian, uint16(100))
	_ = binary.Write(&update, binary.BigEndian, uint16(200))

	serverInit := encodeServerInit("d", 32)
	var serverStream bytes.Buffer
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update.Bytes())

	toClient, _, _, err := runParser(t, ctx, rfb38NoneHandshakeClientBytes(1), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if !bytes.Equal(toClient, serverStream.Bytes()) {
		t.Fatalf("CopyRect not forwarded byte-exact\nwant: %x\ngot:  %x", serverStream.Bytes(), toClient)
	}
}

func TestFramebufferUpdateHextileForwarded(t *testing.T) {
	// One 16x16 Hextile tile with sub-encoding = BackgroundSpecified |
	// AnySubrects (no color) and a single subrect.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var rect bytes.Buffer
	// Tile sub-encoding mask: bg-specified + any-subrects.
	_ = rect.WriteByte(hextileBackgroundSpecified | hextileAnySubrects)
	// Background pixel: 4 bytes (32 bpp).
	_, _ = rect.Write([]byte{0x11, 0x22, 0x33, 0x44})
	// Subrect count.
	_ = rect.WriteByte(1)
	// Single subrect: 2 bytes (x,y,w,h packed).
	_, _ = rect.Write([]byte{0xAB, 0xCD})

	var update bytes.Buffer
	_ = update.WriteByte(s2cFramebufferUpdate)
	_ = update.WriteByte(0)
	_ = binary.Write(&update, binary.BigEndian, uint16(1))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(16))
	_ = binary.Write(&update, binary.BigEndian, uint16(16))
	_ = binary.Write(&update, binary.BigEndian, uint32(encodingHextile))
	_, _ = update.Write(rect.Bytes())

	serverInit := encodeServerInit("d", 32)
	var serverStream bytes.Buffer
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update.Bytes())

	toClient, _, _, err := runParser(t, ctx, rfb38NoneHandshakeClientBytes(1), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if !bytes.Equal(toClient, serverStream.Bytes()) {
		t.Fatalf("Hextile rectangle not forwarded byte-exact\nwant: %x\ngot:  %x", serverStream.Bytes(), toClient)
	}
}

func TestFramebufferUpdateLastRectTerminates(t *testing.T) {
	// FramebufferUpdate with rectangle count = 0xFFFF (LastRect mode).
	// The server emits one Raw rectangle, then a LastRect sentinel, and
	// the parser must stop without trying to read more rectangles. We
	// then send a ServerCutText to prove the parser is back in sync at
	// the message boundary; the parser should drop it and not throw a
	// framing error. Without LastRect handling the parser would keep
	// consuming rectangle headers and desync.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var update bytes.Buffer
	_ = update.WriteByte(s2cFramebufferUpdate)
	_ = update.WriteByte(0)                                     // padding
	_ = binary.Write(&update, binary.BigEndian, uint16(0xFFFF)) // LastRect-mode count
	// Real Raw rectangle 1x1 at 32 bpp = 4-byte body.
	_ = binary.Write(&update, binary.BigEndian, uint16(0))           // x
	_ = binary.Write(&update, binary.BigEndian, uint16(0))           // y
	_ = binary.Write(&update, binary.BigEndian, uint16(1))           // w
	_ = binary.Write(&update, binary.BigEndian, uint16(1))           // h
	_ = binary.Write(&update, binary.BigEndian, uint32(encodingRaw)) // encoding
	_, _ = update.Write([]byte{0x10, 0x20, 0x30, 0x40})
	// LastRect sentinel: zero w/h, encoding -224. Body length is 0.
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_, _ = update.Write([]byte{0xFF, 0xFF, 0xFF, 0x20}) // encodingLastRect (-224)

	// ServerCutText with payload "abc" should be dropped, proving the
	// parser resynced at the FramebufferUpdate boundary.
	var cut bytes.Buffer
	_ = cut.WriteByte(s2cServerCutText)
	_, _ = cut.Write([]byte{0, 0, 0})
	_ = binary.Write(&cut, binary.BigEndian, uint32(3))
	_, _ = cut.WriteString("abc")

	serverInit := encodeServerInit("d", 32)
	var serverStream bytes.Buffer
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update.Bytes())
	_, _ = serverStream.Write(cut.Bytes())

	toClient, _, drops, err := runParser(t, ctx, rfb38NoneHandshakeClientBytes(1), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 || drops[0].Direction != DirServerToClient || drops[0].MessageType != s2cServerCutText {
		t.Fatalf("want one S2C CutText drop, got %#v", drops)
	}
	// The full FramebufferUpdate (including LastRect) must reach the
	// client; the CutText must not.
	want := append([]byte{}, rfb38NoneHandshakeServerBytes(serverInit)...)
	want = append(want, update.Bytes()...)
	if !bytes.Equal(toClient, want) {
		t.Fatalf("LastRect FramebufferUpdate not forwarded byte-exact\nwant: %x\ngot:  %x", want, toClient)
	}
}

func TestFramebufferUpdateLastRectMidStream(t *testing.T) {
	// Server announces 5 rectangles but emits LastRect as the second.
	// Parser must stop after LastRect and treat the remaining count as
	// satisfied, mirroring real viewer behavior. A trailing CutText
	// proves the message boundary is intact.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var update bytes.Buffer
	_ = update.WriteByte(s2cFramebufferUpdate)
	_ = update.WriteByte(0)
	_ = binary.Write(&update, binary.BigEndian, uint16(5)) // overstated count
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(1))
	_ = binary.Write(&update, binary.BigEndian, uint16(1))
	_ = binary.Write(&update, binary.BigEndian, uint32(encodingRaw))
	_, _ = update.Write([]byte{0x10, 0x20, 0x30, 0x40})
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_ = binary.Write(&update, binary.BigEndian, uint16(0))
	_, _ = update.Write([]byte{0xFF, 0xFF, 0xFF, 0x20}) // encodingLastRect (-224)

	var cut bytes.Buffer
	_ = cut.WriteByte(s2cServerCutText)
	_, _ = cut.Write([]byte{0, 0, 0})
	_ = binary.Write(&cut, binary.BigEndian, uint32(0))

	serverInit := encodeServerInit("d", 32)
	var serverStream bytes.Buffer
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update.Bytes())
	_, _ = serverStream.Write(cut.Bytes())

	toClient, _, drops, err := runParser(t, ctx, rfb38NoneHandshakeClientBytes(1), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 || drops[0].Direction != DirServerToClient {
		t.Fatalf("want one S2C drop, got %#v", drops)
	}
	want := append([]byte{}, rfb38NoneHandshakeServerBytes(serverInit)...)
	want = append(want, update.Bytes()...)
	if !bytes.Equal(toClient, want) {
		t.Fatalf("FramebufferUpdate truncated by LastRect not forwarded byte-exact\nwant: %x\ngot:  %x", want, toClient)
	}
}

func TestServerCutTextBetweenFramebufferUpdates(t *testing.T) {
	// A FramebufferUpdate, then ServerCutText (drop), then another
	// FramebufferUpdate. The two updates must reach the client; the
	// CutText must not.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	makeCopyRectUpdate := func(srcX, srcY uint16) []byte {
		var u bytes.Buffer
		_ = u.WriteByte(s2cFramebufferUpdate)
		_ = u.WriteByte(0)
		_ = binary.Write(&u, binary.BigEndian, uint16(1))
		_ = binary.Write(&u, binary.BigEndian, uint16(0))
		_ = binary.Write(&u, binary.BigEndian, uint16(0))
		_ = binary.Write(&u, binary.BigEndian, uint16(10))
		_ = binary.Write(&u, binary.BigEndian, uint16(10))
		_ = binary.Write(&u, binary.BigEndian, uint32(encodingCopyRect))
		_ = binary.Write(&u, binary.BigEndian, srcX)
		_ = binary.Write(&u, binary.BigEndian, srcY)
		return u.Bytes()
	}

	update1 := makeCopyRectUpdate(1, 2)
	update2 := makeCopyRectUpdate(3, 4)
	cut := []byte("dropme")

	var serverStream bytes.Buffer
	serverInit := encodeServerInit("d", 32)
	_, _ = serverStream.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = serverStream.Write(update1)
	_ = serverStream.WriteByte(s2cServerCutText)
	_, _ = serverStream.Write([]byte{0, 0, 0})
	_ = binary.Write(&serverStream, binary.BigEndian, uint32(len(cut))) //nolint:gosec // bounded by RFB protocol field widths
	_, _ = serverStream.Write(cut)
	_, _ = serverStream.Write(update2)

	toClient, _, drops, err := runParser(t, ctx, rfb38NoneHandshakeClientBytes(1), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if len(drops) != 1 || drops[0].PayloadLen != len(cut) {
		t.Fatalf("expected 1 drop with payloadLen=%d, got %v", len(cut), drops)
	}
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeServerBytes(serverInit))
	_, _ = want.Write(update1)
	_, _ = want.Write(update2)
	if !bytes.Equal(toClient, want.Bytes()) {
		t.Fatalf("client missing or got extra bytes around dropped CutText\nwant: %x\ngot:  %x", want.Bytes(), toClient)
	}
}

func TestSplitReadsRobustness(t *testing.T) {
	// Wrap every read in a slowReader that returns one byte per call.
	// The parser must still produce the same output byte-for-byte.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Build a mixed stream: handshake, ClientCutText, KeyEvent.
	cutText := []byte("split")
	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38NoneHandshakeClientBytes(1))
	_ = clientStream.WriteByte(c2sClientCutText)
	_, _ = clientStream.Write([]byte{0, 0, 0})
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(len(cutText))) //nolint:gosec // bounded by RFB protocol field widths
	_, _ = clientStream.Write(cutText)
	_ = clientStream.WriteByte(c2sKeyEvent)
	_ = clientStream.WriteByte(1)
	_, _ = clientStream.Write([]byte{0, 0})
	_ = binary.Write(&clientStream, binary.BigEndian, uint32(0x42))

	serverInit := encodeServerInit("d", 32)
	serverStream := rfb38NoneHandshakeServerBytes(serverInit)

	clientReader := &slowReader{r: bytes.NewReader(clientStream.Bytes())}
	serverReader := &slowReader{r: bytes.NewReader(serverStream)}
	var clientWritten, serverWritten bytes.Buffer

	clientConn := newFakeConn(clientReader, &clientWritten)
	serverConn := newFakeConn(serverReader, &serverWritten)

	var drops []DropEvent
	var dropMu sync.Mutex
	done := make(chan error, 1)
	go func() {
		done <- BicopyDropClipboard(ctx, clientConn, serverConn, func(ev DropEvent) {
			dropMu.Lock()
			drops = append(drops, ev)
			dropMu.Unlock()
		})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("BicopyDropClipboard: %v", err)
		}
	case <-time.After(testutil.WaitShort):
		_ = clientConn.Close()
		_ = serverConn.Close()
		<-done
		t.Fatal("BicopyDropClipboard timed out")
	}

	if len(drops) != 1 {
		t.Fatalf("expected 1 drop, got %d", len(drops))
	}
	var want bytes.Buffer
	_, _ = want.Write(rfb38NoneHandshakeClientBytes(1))
	_ = want.WriteByte(c2sKeyEvent)
	_ = want.WriteByte(1)
	_, _ = want.Write([]byte{0, 0})
	_ = binary.Write(&want, binary.BigEndian, uint32(0x42))
	if !bytes.Equal(serverWritten.Bytes(), want.Bytes()) {
		t.Fatalf("split reads produced wrong server output\nwant: %x\ngot:  %x", want.Bytes(), serverWritten.Bytes())
	}
}

// slowReader returns at most one byte per Read call. Used to verify the
// parser tolerates fragmented reads from the underlying connection.
type slowReader struct {
	r io.Reader
}

func (s *slowReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	return s.r.Read(p[:1])
}

func TestSetPixelFormatUpdatesBPP(t *testing.T) {
	// SetPixelFormat changes the pixel depth mid-stream. A subsequent
	// FramebufferUpdate must use the new bpp for body sizing. We use
	// net.Pipe so the test can synchronize on "the parser observed
	// SetPixelFormat" before staging the update.
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	clientPipe1, clientPipe2 := net.Pipe()
	serverPipe1, serverPipe2 := net.Pipe()
	t.Cleanup(func() {
		_ = clientPipe1.Close()
		_ = clientPipe2.Close()
		_ = serverPipe1.Close()
		_ = serverPipe2.Close()
	})

	parserDone := make(chan error, 1)
	go func() {
		parserDone <- BicopyDropClipboard(ctx, clientPipe2, serverPipe2, nil)
	}()

	// Push the handshake through both directions concurrently.
	serverInit := encodeServerInit("d", 32)
	serverHandshake := rfb38NoneHandshakeServerBytes(serverInit)
	clientHandshake := rfb38NoneHandshakeClientBytes(1)
	writeErrs := make(chan error, 2)
	go func() {
		_, err := serverPipe1.Write(serverHandshake)
		writeErrs <- err
	}()
	go func() {
		_, err := clientPipe1.Write(clientHandshake)
		writeErrs <- err
	}()
	rcvServerCh := make(chan error, 1)
	rcvClientCh := make(chan error, 1)
	go func() {
		buf := make([]byte, len(serverHandshake))
		_, err := io.ReadFull(clientPipe1, buf)
		rcvServerCh <- err
	}()
	go func() {
		buf := make([]byte, len(clientHandshake))
		_, err := io.ReadFull(serverPipe1, buf)
		rcvClientCh <- err
	}()
	if err := <-rcvServerCh; err != nil {
		t.Fatalf("drain server handshake: %v", err)
	}
	if err := <-rcvClientCh; err != nil {
		t.Fatalf("drain client handshake: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := <-writeErrs; err != nil {
			t.Fatalf("handshake write: %v", err)
		}
	}

	// Send SetPixelFormat(bpp=16) C2S, then drain it on the server
	// side. Once the server-side read returns, we know the parser has
	// observed and forwarded the message.
	var setPF bytes.Buffer
	_ = setPF.WriteByte(c2sSetPixelFormat)
	_, _ = setPF.Write([]byte{0, 0, 0})
	_ = setPF.WriteByte(16) // bpp
	_ = setPF.WriteByte(16) // depth
	_ = setPF.WriteByte(0)
	_ = setPF.WriteByte(1)
	_ = binary.Write(&setPF, binary.BigEndian, uint16(31))
	_ = binary.Write(&setPF, binary.BigEndian, uint16(63))
	_ = binary.Write(&setPF, binary.BigEndian, uint16(31))
	_ = setPF.WriteByte(11)
	_ = setPF.WriteByte(5)
	_ = setPF.WriteByte(0)
	_, _ = setPF.Write([]byte{0, 0, 0})
	go func() {
		_, err := clientPipe1.Write(setPF.Bytes())
		writeErrs <- err
	}()
	gotPF := make([]byte, setPF.Len())
	if _, err := io.ReadFull(serverPipe1, gotPF); err != nil {
		t.Fatalf("drain SetPixelFormat: %v", err)
	}
	if err := <-writeErrs; err != nil {
		t.Fatalf("write SetPixelFormat: %v", err)
	}

	// Now stage a 1x1 Raw FramebufferUpdate at 16 bpp = 2 byte body.
	var fbu bytes.Buffer
	_ = fbu.WriteByte(s2cFramebufferUpdate)
	_ = fbu.WriteByte(0)
	_ = binary.Write(&fbu, binary.BigEndian, uint16(1))
	_ = binary.Write(&fbu, binary.BigEndian, uint16(0))
	_ = binary.Write(&fbu, binary.BigEndian, uint16(0))
	_ = binary.Write(&fbu, binary.BigEndian, uint16(1))
	_ = binary.Write(&fbu, binary.BigEndian, uint16(1))
	_ = binary.Write(&fbu, binary.BigEndian, uint32(encodingRaw))
	_, _ = fbu.Write([]byte{0xAA, 0xBB})
	go func() {
		_, err := serverPipe1.Write(fbu.Bytes())
		writeErrs <- err
	}()
	gotFBU := make([]byte, fbu.Len())
	if _, err := io.ReadFull(clientPipe1, gotFBU); err != nil {
		t.Fatalf("drain FramebufferUpdate: %v", err)
	}
	if err := <-writeErrs; err != nil {
		t.Fatalf("write FramebufferUpdate: %v", err)
	}
	if !bytes.Equal(gotFBU, fbu.Bytes()) {
		t.Fatalf("bpp=16 raw rectangle not byte-exact\nwant: %x\ngot:  %x", fbu.Bytes(), gotFBU)
	}

	// Close everything to release the parser.
	_ = clientPipe1.Close()
	_ = serverPipe1.Close()
	select {
	case <-parserDone:
	case <-time.After(testutil.WaitShort):
		t.Fatal("parser did not return after pipe close")
	}
}

// TestParserLowerVersionNegotiation verifies the parser uses the lower
// of the two ProtocolVersion strings, per RFC 6143 §7.1.1.
func TestParserLowerVersionNegotiation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	// Server speaks 3.8, client speaks 3.7. Negotiated version is 3.7,
	// which means RFB 3.7 omits SecurityResult after a None choice.
	serverVersion := []byte("RFB 003.008\n")
	clientVersion := []byte("RFB 003.007\n")

	var serverStream bytes.Buffer
	_, _ = serverStream.Write(serverVersion)
	_ = serverStream.WriteByte(1) // 1 security type
	_ = serverStream.WriteByte(1) // None
	// No SecurityResult.
	serverInit := encodeServerInit("d", 32)
	_, _ = serverStream.Write(serverInit)

	var clientStream bytes.Buffer
	_, _ = clientStream.Write(clientVersion)
	_ = clientStream.WriteByte(1) // choose None
	_ = clientStream.WriteByte(1) // ClientInit shared

	toClient, toServer, _, err := runParser(t, ctx, clientStream.Bytes(), serverStream.Bytes())
	if err != nil {
		t.Fatalf("BicopyDropClipboard: %v", err)
	}
	if !bytes.Equal(toClient, serverStream.Bytes()) {
		t.Fatalf("client received unexpected bytes; rfb 3.7 SecurityResult must be omitted")
	}
	if !bytes.Equal(toServer, clientStream.Bytes()) {
		t.Fatalf("server received unexpected bytes")
	}
}

func TestRejectsUnsupportedRFBVersion(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	serverStream := []byte("RFB 005.000\n")
	clientStream := []byte("RFB 003.008\n")
	_, _, _, err := runParser(t, ctx, clientStream, serverStream) //nolint:dogsled // runParser returns four values; only the error matters here.
	if err == nil {
		t.Fatal("expected error for unsupported RFB version")
	}
}

func TestRejectsUnsupportedSecurityType(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	var serverStream bytes.Buffer
	_, _ = serverStream.Write(rfb38ServerVersion)
	_ = serverStream.WriteByte(1)
	_ = serverStream.WriteByte(19) // VeNCrypt: not supported.

	var clientStream bytes.Buffer
	_, _ = clientStream.Write(rfb38ClientVersion)
	_ = clientStream.WriteByte(19) // pick VeNCrypt

	_, _, _, err := runParser(t, ctx, clientStream.Bytes(), serverStream.Bytes()) //nolint:dogsled // runParser returns four values; only the error matters here.
	if err == nil {
		t.Fatal("expected error for VeNCrypt")
	}
}

func TestCutTextPayloadLenMinInt32Rejected(t *testing.T) {
	t.Parallel()
	buf := make([]byte, 4)
	var minVal int32 = math.MinInt32
	binary.BigEndian.PutUint32(buf, uint32(minVal)) //nolint:gosec // bounded by RFB protocol field widths
	if _, err := cutTextPayloadLen(buf); err == nil {
		t.Fatal("expected error for math.MinInt32 length")
	}
}

func TestCutTextPayloadLenExtendedClipboardNegated(t *testing.T) {
	t.Parallel()
	// Extended clipboard sets the high bit. Length -100 means "100
	// bytes of extended clipboard payload."
	buf := make([]byte, 4)
	var negLen int32 = -100
	binary.BigEndian.PutUint32(buf, uint32(negLen)) //nolint:gosec // bounded by RFB protocol field widths
	got, err := cutTextPayloadLen(buf)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 100 {
		t.Errorf("payload len = %d, want 100", got)
	}
}

// TestNetPipeIntegration runs the parser over a real net.Pipe pair to
// catch interactions between the goroutines that pre-buffered byte
// slices hide. This is closer to how the live proxy uses the function.
func TestNetPipeIntegration(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(context.Background(), testutil.WaitShort)
	defer cancel()

	clientPipe1, clientPipe2 := net.Pipe()
	serverPipe1, serverPipe2 := net.Pipe()

	defer clientPipe1.Close()
	defer clientPipe2.Close()
	defer serverPipe1.Close()
	defer serverPipe2.Close()

	// The parser sees pipe2 ends as the connections.
	var drops []DropEvent
	var dropMu sync.Mutex
	parserDone := make(chan error, 1)
	go func() {
		parserDone <- BicopyDropClipboard(ctx, clientPipe2, serverPipe2, func(ev DropEvent) {
			dropMu.Lock()
			drops = append(drops, ev)
			dropMu.Unlock()
		})
	}()

	serverInit := encodeServerInit("d", 32)
	serverHandshake := rfb38NoneHandshakeServerBytes(serverInit)
	clientHandshake := rfb38NoneHandshakeClientBytes(1)

	// net.Pipe is synchronous: writes block until the other side
	// reads. The handshake interleaves S2C and C2S, so the test must
	// drive both sides concurrently with the parser.
	writeErrs := make(chan error, 2)
	go func() {
		_, err := serverPipe1.Write(serverHandshake)
		writeErrs <- err
	}()
	go func() {
		_, err := clientPipe1.Write(clientHandshake)
		writeErrs <- err
	}()

	// Each side must drain the parser's output concurrently with
	// pushing input. Otherwise the parser stalls on its forward writes
	// while we wait for it to consume our writes.
	type readResult struct {
		err error
	}
	clientReadCh := make(chan readResult, 1)
	serverReadCh := make(chan readResult, 1)
	go func() {
		buf := make([]byte, len(serverHandshake))
		_, err := io.ReadFull(clientPipe1, buf)
		clientReadCh <- readResult{err: err}
	}()
	go func() {
		buf := make([]byte, len(clientHandshake))
		_, err := io.ReadFull(serverPipe1, buf)
		serverReadCh <- readResult{err: err}
	}()

	select {
	case r := <-clientReadCh:
		if r.err != nil {
			t.Fatalf("read server handshake on client side: %v", r.err)
		}
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out reading server handshake on client side")
	}
	select {
	case r := <-serverReadCh:
		if r.err != nil {
			t.Fatalf("read client handshake on server side: %v", r.err)
		}
	case <-time.After(testutil.WaitShort):
		t.Fatal("timed out reading client handshake on server side")
	}
	for i := 0; i < 2; i++ {
		if err := <-writeErrs; err != nil {
			t.Fatalf("handshake write: %v", err)
		}
	}

	// Client now sends a ClientCutText.
	cutText := []byte("payload")
	var ct bytes.Buffer
	_ = ct.WriteByte(c2sClientCutText)
	_, _ = ct.Write([]byte{0, 0, 0})
	_ = binary.Write(&ct, binary.BigEndian, uint32(len(cutText))) //nolint:gosec // bounded by RFB protocol field widths
	_, _ = ct.Write(cutText)
	if _, err := clientPipe1.Write(ct.Bytes()); err != nil {
		t.Fatalf("write CutText: %v", err)
	}
	// Followed by a KeyEvent that the server must observe.
	var ke bytes.Buffer
	_ = ke.WriteByte(c2sKeyEvent)
	_ = ke.WriteByte(1)
	_, _ = ke.Write([]byte{0, 0})
	_ = binary.Write(&ke, binary.BigEndian, uint32(0x99))
	if _, err := clientPipe1.Write(ke.Bytes()); err != nil {
		t.Fatalf("write KeyEvent: %v", err)
	}

	// Server side reads only the KeyEvent (CutText was dropped).
	gotKE := make([]byte, ke.Len())
	if _, err := io.ReadFull(serverPipe1, gotKE); err != nil {
		t.Fatalf("read KeyEvent on server side: %v", err)
	}
	if !bytes.Equal(gotKE, ke.Bytes()) {
		t.Fatalf("KeyEvent corrupted by parser\nwant: %x\ngot:  %x", ke.Bytes(), gotKE)
	}

	// Close both client-side ends to unblock the parser, then collect
	// its result.
	_ = clientPipe1.Close()
	_ = serverPipe1.Close()
	select {
	case err := <-parserDone:
		// io.EOF from one side is fine; from a Close on the other side
		// is also fine. We only expect not-EOF errors here.
		if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
			// net.Pipe ErrClosedPipe wraps differently in different
			// Go versions; tolerate either.
			t.Logf("parser returned: %v", err)
		}
	case <-time.After(testutil.WaitShort):
		t.Fatal("parser did not return after pipe close")
	}

	dropMu.Lock()
	defer dropMu.Unlock()
	if len(drops) != 1 || drops[0].PayloadLen != len(cutText) {
		t.Fatalf("expected 1 drop with payloadLen=%d, got %v", len(cutText), drops)
	}
}

func TestIsMessageBoundaryEOFAcceptsCleanCloseSignals(t *testing.T) {
	// Catalog every error variant the live proxy treats as a clean
	// close at the start of the next message. EOF is the canonical
	// case; the others surface when a Coder dashboard tab disconnects
	// (request context cancel, deadline) or when we close the
	// underlying conn ourselves (net.ErrClosed). Wrapping is via
	// fmt.Errorf so errors.Is must see through it.
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"io.EOF", io.EOF, true},
		{"wrapped io.EOF", xerrors.Errorf("read: %w", io.EOF), true},
		{"unexpected EOF", io.ErrUnexpectedEOF, false},
		{"context.Canceled", context.Canceled, true},
		{"wrapped context.Canceled", xerrors.Errorf("websocket gone: %w", context.Canceled), true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"net.ErrClosed", net.ErrClosed, true},
		{"wrapped net.ErrClosed", xerrors.Errorf("conn: %w", net.ErrClosed), true},
		{"plain error", xerrors.New("nope"), false},
	}
	for _, tc := range cases {
		got := isMessageBoundaryEOF(tc.err)
		if got != tc.want {
			t.Errorf("isMessageBoundaryEOF(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}
