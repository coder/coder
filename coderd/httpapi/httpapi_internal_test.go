package httpapi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
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

type oneWaySocketWriter struct {
	*httptest.ResponseRecorder
	serverConn       net.Conn
	clientConn       net.Conn
	serverReadWriter *bufio.ReadWriter
	testContext      *testing.T
}

func (m oneWaySocketWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.serverReadWriter, nil
}

func (m oneWaySocketWriter) Flush() {
	err := m.serverReadWriter.Flush()
	require.NoError(m.testContext, err)
}

func (m oneWaySocketWriter) Write(b []byte) (int, error) {
	return m.serverReadWriter.Write(b)
}

func newOneWayRequest(ctx context.Context, t *testing.T) *http.Request {
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

func newOneWayWriter(t *testing.T) oneWaySocketWriter {
	t.Helper()

	mockServer, mockClient := net.Pipe()
	t.Cleanup(func() {
		_ = mockServer.Close()
		_ = mockClient.Close()
	})

	return oneWaySocketWriter{
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

func readWebSocketFrame(t *testing.T, conn net.Conn) (byte, []byte) {
	t.Helper()
	require.NoError(t, conn.SetReadDeadline(time.Now().Add(testutil.WaitShort)))

	header := make([]byte, 2)
	_, err := io.ReadFull(conn, header)
	require.NoError(t, err)

	payloadLength := int(header[1] & 0x7f)
	require.Less(t, payloadLength, 126)

	payload := make([]byte, payloadLength)
	_, err = io.ReadFull(conn, payload)
	require.NoError(t, err)
	return header[0] & 0x0f, payload
}

func writeWebSocketTextFrame(t *testing.T, conn net.Conn, payload []byte) {
	t.Helper()
	require.Less(t, len(payload), 126)

	mask := [4]byte{1, 2, 3, 4}
	frame := []byte{0x81, 0x80 | byte(len(payload))}
	frame = append(frame, mask[:]...)
	for i, b := range payload {
		frame = append(frame, b^mask[i%len(mask)])
	}
	_, err := conn.Write(frame)
	require.NoError(t, err)
}

func requireClosed(ctx context.Context, t *testing.T, done <-chan struct{}) {
	t.Helper()

	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for connection to close")
	}
}

func TestOneWayWebSocketEventSender(t *testing.T) {
	t.Parallel()

	t.Run("Produces error if the socket connection could not be established", func(t *testing.T) {
		t.Parallel()

		incorrectProtocols := []struct {
			major int
			minor int
			proto string
		}{
			{0, 9, "HTTP/0.9"},
			{1, 0, "HTTP/1.0"},
		}
		for _, p := range incorrectProtocols {
			ctx := testutil.Context(t, testutil.WaitShort)
			req := newOneWayRequest(ctx, t)
			req.ProtoMajor = p.major
			req.ProtoMinor = p.minor
			req.Proto = p.proto

			writer := newOneWayWriter(t)
			_, _, err := OneWayWebSocketEventSender(slogtest.Make(t, nil))(writer, req)
			require.ErrorContains(t, err, p.proto)
		}
	})

	t.Run("Returned callback can publish new event to WebSocket connection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newOneWayRequest(ctx, t)
		writer := newOneWayWriter(t)
		send, _, err := OneWayWebSocketEventSender(slogtest.Make(t, nil))(writer, req)
		require.NoError(t, err)

		serverPayload := codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Blah",
		}
		err = send(serverPayload)
		require.NoError(t, err)

		serverBytes, err := json.Marshal(serverPayload)
		require.NoError(t, err)
		_, clientPayload := readWebSocketFrame(t, writer.clientConn)
		require.True(t, bytes.Contains(clientPayload, serverBytes))
	})

	t.Run("Signals to outside consumer when socket has been closed", func(t *testing.T) {
		t.Parallel()

		timeoutCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(timeoutCtx)
		req := newOneWayRequest(ctx, t)
		writer := newOneWayWriter(t)
		_, done, err := OneWayWebSocketEventSender(slogtest.Make(t, nil))(writer, req)
		require.NoError(t, err)

		cancel()
		require.NoError(t, writer.clientConn.Close())
		requireClosed(timeoutCtx, t, done)
	})

	t.Run("Socket will immediately close if client sends any message", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newOneWayRequest(ctx, t)
		writer := newOneWayWriter(t)
		_, done, err := OneWayWebSocketEventSender(slogtest.Make(t, nil))(writer, req)
		require.NoError(t, err)

		type JunkClientEvent struct {
			Value string
		}
		b, err := json.Marshal(JunkClientEvent{"Hi :)"})
		require.NoError(t, err)
		writeWebSocketTextFrame(t, writer.clientConn, b)
		require.NoError(t, writer.clientConn.Close())
		requireClosed(ctx, t, done)
	})

	t.Run("Renders the socket inert if the request context cancels", func(t *testing.T) {
		t.Parallel()

		timeoutCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(timeoutCtx)
		req := newOneWayRequest(ctx, t)
		writer := newOneWayWriter(t)
		send, done, err := OneWayWebSocketEventSender(slogtest.Make(t, nil))(writer, req)
		require.NoError(t, err)

		cancel()
		require.NoError(t, writer.clientConn.Close())
		requireClosed(timeoutCtx, t, done)
		err = send(codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Didn't realize you were closed - sorry! I'll try coming back tomorrow.",
		})
		require.Equal(t, err, ctx.Err())
		_, open := <-done
		require.False(t, open)
		_, err = writer.serverConn.Write([]byte{})
		require.Equal(t, err, io.ErrClosedPipe)
		_, err = writer.clientConn.Read([]byte{})
		require.Error(t, err)
	})

	t.Run("Sends a heartbeat to the socket on a fixed interval of time to keep connections alive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("HeartbeatClose")
		defer trap.Close()

		req := newOneWayRequest(ctx, t)
		writer := newOneWayWriter(t)
		_, _, err := oneWayWebSocketEventSenderWith(
			slogtest.Make(t, nil),
			mClock,
			time.Second,
		)(writer, req)
		require.NoError(t, err)

		trap.MustWait(ctx).MustRelease(ctx)
		mClock.Advance(time.Second).MustWait(ctx)
		opcode, _ := readWebSocketFrame(t, writer.clientConn)
		require.Equal(t, byte(0x9), opcode)
	})
}

type serverSentWriter struct {
	*httptest.ResponseRecorder
	flushed chan string
}

func (m *serverSentWriter) Flush() {
	m.ResponseRecorder.Flush()
	m.flushed <- m.Body.String()
	m.Body.Reset()
}

func newServerSentRequest(ctx context.Context, t *testing.T) *http.Request {
	t.Helper()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "ws://www.fake-website.com/logs", nil)
	require.NoError(t, err)
	return req
}

func newServerSentWriter() *serverSentWriter {
	return &serverSentWriter{ResponseRecorder: httptest.NewRecorder(), flushed: make(chan string)}
}

func requireServerSentFlush(ctx context.Context, t *testing.T, writer *serverSentWriter) string {
	t.Helper()

	select {
	case payload := <-writer.flushed:
		return payload
	case <-ctx.Done():
		t.Fatal("timed out waiting for server-sent event flush")
		return ""
	}
}

func TestServerSentEventSender(t *testing.T) {
	t.Parallel()

	t.Run("Mutates response headers to support SSE connections", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newServerSentRequest(ctx, t)
		writer := newServerSentWriter()
		_, _, err := ServerSentEventSender(writer, req)
		require.NoError(t, err)

		h := writer.Header()
		require.Equal(t, h.Get("Content-Type"), "text/event-stream")
		require.Equal(t, h.Get("Cache-Control"), "no-cache")
		require.Equal(t, h.Get("Connection"), "keep-alive")
		require.Equal(t, h.Get("X-Accel-Buffering"), "no")
	})

	t.Run("Returned callback can publish new event to SSE connection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newServerSentRequest(ctx, t)
		writer := newServerSentWriter()
		send, _, err := ServerSentEventSender(writer, req)
		require.NoError(t, err)

		serverPayload := codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Blah",
		}
		err = send(serverPayload)
		require.NoError(t, err)

		require.Equal(t, "event: data\ndata: \"Blah\"\n\n", requireServerSentFlush(ctx, t, writer))
	})

	t.Run("Signals to outside consumer when connection has been closed", func(t *testing.T) {
		t.Parallel()

		timeoutCtx := testutil.Context(t, testutil.WaitShort)
		ctx, cancel := context.WithCancel(timeoutCtx)
		req := newServerSentRequest(ctx, t)
		writer := newServerSentWriter()
		_, done, err := ServerSentEventSender(writer, req)
		require.NoError(t, err)

		cancel()
		requireClosed(timeoutCtx, t, done)
	})

	t.Run("Sends a heartbeat to the client on a fixed interval of time to keep connections alive", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		mClock := quartz.NewMock(t)
		trap := mClock.Trap().NewTicker("ServerSentEventSender")
		defer trap.Close()

		req := newServerSentRequest(ctx, t)
		writer := newServerSentWriter()
		_, _, err := serverSentEventSenderWith(writer, req, mClock, time.Second)
		require.NoError(t, err)

		trap.MustWait(ctx).MustRelease(ctx)

		expected := "event: " + string(codersdk.ServerSentEventTypePing) + "\n\n"
		for range 3 {
			mClock.Advance(time.Second).MustWait(ctx)
			require.Equal(t, expected, requireServerSentFlush(ctx, t, writer))
		}
	})
}
