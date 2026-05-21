package httpapi

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

type oneWayHeartbeatWriter struct {
	*httptest.ResponseRecorder
	serverConn       net.Conn
	clientConn       net.Conn
	serverReadWriter *bufio.ReadWriter
	testContext      *testing.T
}

func (m oneWayHeartbeatWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.serverReadWriter, nil
}

func (m oneWayHeartbeatWriter) Flush() {
	err := m.serverReadWriter.Flush()
	require.NoError(m.testContext, err)
}

func (m oneWayHeartbeatWriter) Write(b []byte) (int, error) {
	return m.serverReadWriter.Write(b)
}

func newOneWayHeartbeatRequest(t *testing.T, ctx context.Context) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "ws://www.fake-website.com/logs", nil)
	require.NoError(t, err)

	h := req.Header
	h.Add("Connection", "Upgrade")
	h.Add("Upgrade", "websocket")
	h.Add("Sec-WebSocket-Version", "13")
	h.Add("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	return req
}

func newOneWayHeartbeatWriter(t *testing.T) oneWayHeartbeatWriter {
	t.Helper()

	mockServer, mockClient := net.Pipe()
	t.Cleanup(func() {
		_ = mockServer.Close()
		_ = mockClient.Close()
	})

	return oneWayHeartbeatWriter{
		testContext:      t,
		serverConn:       mockServer,
		clientConn:       mockClient,
		ResponseRecorder: httptest.NewRecorder(),
		serverReadWriter: bufio.NewReadWriter(
			bufio.NewReader(mockServer),
			bufio.NewWriter(mockServer),
		),
	}
}

func readWebSocketOpcode(t *testing.T, conn net.Conn) byte {
	t.Helper()

	header := make([]byte, 2)
	_, err := io.ReadFull(conn, header)
	require.NoError(t, err)

	payloadLength := int(header[1] & 0x7f)
	require.Less(t, payloadLength, 126)

	_, err = io.CopyN(io.Discard, conn, int64(payloadLength))
	require.NoError(t, err)
	return header[0] & 0x0f
}

func TestOneWayWebSocketEventSenderHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTicker("HeartbeatClose")
	defer trap.Close()

	req := newOneWayHeartbeatRequest(t, ctx)
	writer := newOneWayHeartbeatWriter(t)
	_, _, err := oneWayWebSocketEventSenderWith(
		slogtest.Make(t, nil),
		mClock,
		time.Second,
	)(writer, req)
	require.NoError(t, err)

	trap.MustWait(ctx).MustRelease(ctx)
	require.NoError(t, writer.clientConn.SetReadDeadline(time.Now().Add(testutil.WaitShort)))

	mClock.Advance(time.Second).MustWait(ctx)
	require.Equal(t, byte(0x9), readWebSocketOpcode(t, writer.clientConn))
}

type serverSentHeartbeatWriter struct {
	*httptest.ResponseRecorder
	flushed chan string
}

func (m *serverSentHeartbeatWriter) Flush() {
	m.ResponseRecorder.Flush()
	m.flushed <- m.Body.String()
	m.Body.Reset()
}

func TestServerSentEventSenderHeartbeat(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitShort)
	mClock := quartz.NewMock(t)
	trap := mClock.Trap().NewTicker("ServerSentEventSender")
	defer trap.Close()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "ws://www.fake-website.com/logs", nil)
	require.NoError(t, err)
	writer := &serverSentHeartbeatWriter{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan string)}
	_, _, err = serverSentEventSenderWith(writer, req, mClock, time.Second)
	require.NoError(t, err)

	trap.MustWait(ctx).MustRelease(ctx)

	expected := "event: " + string(codersdk.ServerSentEventTypePing) + "\n\n"
	for range 3 {
		mClock.Advance(time.Second).MustWait(ctx)
		select {
		case payload := <-writer.flushed:
			require.Equal(t, expected, payload)
		case <-ctx.Done():
			t.Fatal("timed out waiting for server-sent event heartbeat")
		}
	}
}
