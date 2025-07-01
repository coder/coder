package httpapi_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/testutil"
)

func TestInternalServerError(t *testing.T) {
	t.Parallel()

	t.Run("NoError", func(t *testing.T) {
		t.Parallel()
		w := httptest.NewRecorder()
		httpapi.InternalServerError(w, nil)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.NotEmpty(t, resp.Message)
		require.Empty(t, resp.Detail)
	})

	t.Run("WithError", func(t *testing.T) {
		t.Parallel()
		var (
			w       = httptest.NewRecorder()
			httpErr = xerrors.New("error!")
		)

		httpapi.InternalServerError(w, httpErr)

		var resp codersdk.Response
		err := json.NewDecoder(w.Body).Decode(&resp)
		require.NoError(t, err)
		require.Equal(t, http.StatusInternalServerError, w.Code)
		require.NotEmpty(t, resp.Message)
		require.Equal(t, httpErr.Error(), resp.Detail)
	})
}

func TestWrite(t *testing.T) {
	t.Parallel()
	t.Run("NoErrors", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		rw := httptest.NewRecorder()
		httpapi.Write(ctx, rw, http.StatusOK, codersdk.Response{
			Message: "Wow.",
		})
		var m map[string]interface{}
		err := json.NewDecoder(rw.Body).Decode(&m)
		require.NoError(t, err)
		_, ok := m["errors"]
		require.False(t, ok)
	})
}

func TestRead(t *testing.T) {
	t.Parallel()
	t.Run("EmptyStruct", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString("{}"))
		v := struct{}{}
		require.True(t, httpapi.Read(ctx, rw, r, &v))
	})

	t.Run("NoBody", func(t *testing.T) {
		t.Parallel()
		ctx := context.Background()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", nil)
		var v json.RawMessage
		require.False(t, httpapi.Read(ctx, rw, r, v))
	})

	t.Run("Validate", func(t *testing.T) {
		t.Parallel()
		type toValidate struct {
			Value string `json:"value" validate:"required"`
		}
		ctx := context.Background()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString(`{"value":"hi"}`))

		var validate toValidate
		require.True(t, httpapi.Read(ctx, rw, r, &validate))
		require.Equal(t, "hi", validate.Value)
	})

	t.Run("ValidateFailure", func(t *testing.T) {
		t.Parallel()
		type toValidate struct {
			Value string `json:"value" validate:"required"`
		}
		ctx := context.Background()
		rw := httptest.NewRecorder()
		r := httptest.NewRequest("POST", "/", bytes.NewBufferString("{}"))

		var validate toValidate
		require.False(t, httpapi.Read(ctx, rw, r, &validate))
		var v codersdk.Response
		err := json.NewDecoder(rw.Body).Decode(&v)
		require.NoError(t, err)
		require.Len(t, v.Validations, 1)
		require.Equal(t, "value", v.Validations[0].Field)
		require.Equal(t, "Validation failed for tag \"required\" with value: \"\"", v.Validations[0].Detail)
	})
}

func TestWebsocketCloseMsg(t *testing.T) {
	t.Parallel()

	t.Run("Sprintf", func(t *testing.T) {
		t.Parallel()

		var (
			msg  = "this is my message %q %q"
			opts = []any{"colin", "kyle"}
		)

		expected := fmt.Sprintf(msg, opts...)
		got := httpapi.WebsocketCloseSprintf(msg, opts...)
		assert.Equal(t, expected, got)
	})

	t.Run("TruncateSingleByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("d", 255)
		trunc := httpapi.WebsocketCloseSprintf("%s", msg)
		assert.Equal(t, len(trunc), 123)
	})

	t.Run("TruncateMultiByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("こんにちは", 10)
		trunc := httpapi.WebsocketCloseSprintf("%s", msg)
		assert.Equal(t, len(trunc), 123)
	})
}

// Our WebSocket library accepts any arbitrary ResponseWriter at the type level,
// but the writer must also implement http.Hijacker for long-lived connections.
type mockOneWaySocketWriter struct {
	serverRecorder   *httptest.ResponseRecorder
	serverConn       net.Conn
	clientConn       net.Conn
	serverReadWriter *bufio.ReadWriter
	testContext      *testing.T
}

func (m mockOneWaySocketWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.serverReadWriter, nil
}

func (m mockOneWaySocketWriter) Flush() {
	err := m.serverReadWriter.Flush()
	require.NoError(m.testContext, err)
}

func (m mockOneWaySocketWriter) Header() http.Header {
	return m.serverRecorder.Header()
}

func (m mockOneWaySocketWriter) Write(b []byte) (int, error) {
	return m.serverReadWriter.Write(b)
}

func (m mockOneWaySocketWriter) WriteHeader(code int) {
	m.serverRecorder.WriteHeader(code)
}

type mockEventSenderWrite func(b []byte) (int, error)

func (w mockEventSenderWrite) Write(b []byte) (int, error) {
	return w(b)
}

func TestOneWayWebSocketEventSender(t *testing.T) {
	t.Parallel()

	newBaseRequest := func(ctx context.Context) *http.Request {
		url := "ws://www.fake-website.com/logs"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)

		h := req.Header
		h.Add("Connection", "Upgrade")
		h.Add("Upgrade", "websocket")
		h.Add("Sec-WebSocket-Version", "13")
		h.Add("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==") // Just need any string

		return req
	}

	newOneWayWriter := func(t *testing.T) mockOneWaySocketWriter {
		mockServer, mockClient := net.Pipe()
		recorder := httptest.NewRecorder()

		var write mockEventSenderWrite = func(b []byte) (int, error) {
			serverCount, err := mockServer.Write(b)
			if err != nil {
				return 0, err
			}
			recorderCount, err := recorder.Write(b)
			if err != nil {
				return 0, err
			}
			return min(serverCount, recorderCount), nil
		}

		return mockOneWaySocketWriter{
			testContext:    t,
			serverConn:     mockServer,
			clientConn:     mockClient,
			serverRecorder: recorder,
			serverReadWriter: bufio.NewReadWriter(
				bufio.NewReader(mockServer),
				bufio.NewWriter(write),
			),
		}
	}

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
			req := newBaseRequest(ctx)
			req.ProtoMajor = p.major
			req.ProtoMinor = p.minor
			req.Proto = p.proto

			writer := newOneWayWriter(t)
			_, _, err := httpapi.OneWayWebSocketEventSender(writer, req)
			require.ErrorContains(t, err, p.proto)
		}
	})

	t.Run("Returned callback can publish new event to WebSocket connection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newBaseRequest(ctx)
		writer := newOneWayWriter(t)
		send, _, err := httpapi.OneWayWebSocketEventSender(writer, req)
		require.NoError(t, err)

		serverPayload := codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Blah",
		}
		err = send(serverPayload)
		require.NoError(t, err)

		// The client connection will receive a little bit of additional data on
		// top of the main payload. Have to make sure check has tolerance for
		// extra data being present
		serverBytes, err := json.Marshal(serverPayload)
		require.NoError(t, err)
		clientBytes, err := io.ReadAll(writer.clientConn)
		require.NoError(t, err)
		require.True(t, bytes.Contains(clientBytes, serverBytes))
	})

	t.Run("Signals to outside consumer when socket has been closed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		req := newBaseRequest(ctx)
		writer := newOneWayWriter(t)
		_, done, err := httpapi.OneWayWebSocketEventSender(writer, req)
		require.NoError(t, err)

		successC := make(chan bool)
		ticker := time.NewTicker(testutil.WaitShort)
		go func() {
			select {
			case <-done:
				successC <- true
			case <-ticker.C:
				successC <- false
			}
		}()

		cancel()
		require.True(t, <-successC)
	})

	t.Run("Socket will immediately close if client sends any message", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newBaseRequest(ctx)
		writer := newOneWayWriter(t)
		_, done, err := httpapi.OneWayWebSocketEventSender(writer, req)
		require.NoError(t, err)

		successC := make(chan bool)
		ticker := time.NewTicker(testutil.WaitShort)
		go func() {
			select {
			case <-done:
				successC <- true
			case <-ticker.C:
				successC <- false
			}
		}()

		type JunkClientEvent struct {
			Value string
		}
		b, err := json.Marshal(JunkClientEvent{"Hi :)"})
		require.NoError(t, err)
		_, err = writer.clientConn.Write(b)
		require.NoError(t, err)
		require.True(t, <-successC)
	})

	t.Run("Renders the socket inert if the request context cancels", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		req := newBaseRequest(ctx)
		writer := newOneWayWriter(t)
		send, done, err := httpapi.OneWayWebSocketEventSender(writer, req)
		require.NoError(t, err)

		successC := make(chan bool)
		ticker := time.NewTicker(testutil.WaitShort)
		go func() {
			select {
			case <-done:
				successC <- true
			case <-ticker.C:
				successC <- false
			}
		}()

		cancel()
		require.True(t, <-successC)
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
		require.Equal(t, err, io.EOF)
	})

	t.Run("Sends a heartbeat to the socket on a fixed internal of time to keep connections alive", func(t *testing.T) {
		t.Parallel()

		// Need add at least three heartbeats for something to be reliably
		// counted as an interval, but also need some wiggle room
		heartbeatCount := 3
		hbDuration := time.Duration(heartbeatCount) * httpapi.HeartbeatInterval
		timeout := hbDuration + (5 * time.Second)

		ctx := testutil.Context(t, timeout)
		req := newBaseRequest(ctx)
		writer := newOneWayWriter(t)
		_, _, err := httpapi.OneWayWebSocketEventSender(writer, req)
		require.NoError(t, err)

		type Result struct {
			Err     error
			Success bool
		}
		resultC := make(chan Result)
		go func() {
			err := writer.
				clientConn.
				SetReadDeadline(time.Now().Add(timeout))
			if err != nil {
				resultC <- Result{err, false}
				return
			}
			for range heartbeatCount {
				pingBuffer := make([]byte, 1)
				pingSize, err := writer.clientConn.Read(pingBuffer)
				if err != nil || pingSize != 1 {
					resultC <- Result{err, false}
					return
				}
			}
			resultC <- Result{nil, true}
		}()

		result := <-resultC
		require.NoError(t, result.Err)
		require.True(t, result.Success)
	})
}

// ServerSentEventSender accepts any arbitrary ResponseWriter at the type level,
// but the writer must also implement http.Flusher for long-lived connections
type mockServerSentWriter struct {
	serverRecorder *httptest.ResponseRecorder
	serverConn     net.Conn
	clientConn     net.Conn
	buffer         *bytes.Buffer
	testContext    *testing.T
}

func (m mockServerSentWriter) Flush() {
	b := m.buffer.Bytes()
	_, err := m.serverConn.Write(b)
	require.NoError(m.testContext, err)
	m.buffer.Reset()

	// Must close server connection to indicate EOF for any reads from the
	// client connection; otherwise reads block forever. This is a testing
	// limitation compared to the one-way websockets, since we have no way to
	// frame the data and auto-indicate EOF for each message
	err = m.serverConn.Close()
	require.NoError(m.testContext, err)
}

func (m mockServerSentWriter) Header() http.Header {
	return m.serverRecorder.Header()
}

func (m mockServerSentWriter) Write(b []byte) (int, error) {
	return m.buffer.Write(b)
}

func (m mockServerSentWriter) WriteHeader(code int) {
	m.serverRecorder.WriteHeader(code)
}

func TestServerSentEventSender(t *testing.T) {
	t.Parallel()

	newBaseRequest := func(ctx context.Context) *http.Request {
		url := "ws://www.fake-website.com/logs"
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		return req
	}

	newServerSentWriter := func(t *testing.T) mockServerSentWriter {
		mockServer, mockClient := net.Pipe()
		return mockServerSentWriter{
			testContext:    t,
			serverRecorder: httptest.NewRecorder(),
			clientConn:     mockClient,
			serverConn:     mockServer,
			buffer:         &bytes.Buffer{},
		}
	}

	t.Run("Mutates response headers to support SSE connections", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newBaseRequest(ctx)
		writer := newServerSentWriter(t)
		_, _, err := httpapi.ServerSentEventSender(writer, req)
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
		req := newBaseRequest(ctx)
		writer := newServerSentWriter(t)
		send, _, err := httpapi.ServerSentEventSender(writer, req)
		require.NoError(t, err)

		serverPayload := codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Blah",
		}
		err = send(serverPayload)
		require.NoError(t, err)

		clientBytes, err := io.ReadAll(writer.clientConn)
		require.NoError(t, err)
		require.Equal(
			t,
			string(clientBytes),
			"event: data\ndata: \"Blah\"\n\n",
		)
	})

	t.Run("Signals to outside consumer when connection has been closed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		req := newBaseRequest(ctx)
		writer := newServerSentWriter(t)
		_, done, err := httpapi.ServerSentEventSender(writer, req)
		require.NoError(t, err)

		successC := make(chan bool)
		ticker := time.NewTicker(testutil.WaitShort)
		go func() {
			select {
			case <-done:
				successC <- true
			case <-ticker.C:
				successC <- false
			}
		}()

		cancel()
		require.True(t, <-successC)
	})

	t.Run("Sends a heartbeat to the client on a fixed internal of time to keep connections alive", func(t *testing.T) {
		t.Parallel()

		// Need add at least three heartbeats for something to be reliably
		// counted as an interval, but also need some wiggle room
		heartbeatCount := 3
		hbDuration := time.Duration(heartbeatCount) * httpapi.HeartbeatInterval
		timeout := hbDuration + (5 * time.Second)

		ctx := testutil.Context(t, timeout)
		req := newBaseRequest(ctx)
		writer := newServerSentWriter(t)
		_, _, err := httpapi.ServerSentEventSender(writer, req)
		require.NoError(t, err)

		type Result struct {
			Err     error
			Success bool
		}
		resultC := make(chan Result)
		go func() {
			err := writer.
				clientConn.
				SetReadDeadline(time.Now().Add(timeout))
			if err != nil {
				resultC <- Result{err, false}
				return
			}
			for range heartbeatCount {
				pingBuffer := make([]byte, 1)
				pingSize, err := writer.clientConn.Read(pingBuffer)
				if err != nil || pingSize != 1 {
					resultC <- Result{err, false}
					return
				}
			}
			resultC <- Result{nil, true}
		}()

		result := <-resultC
		require.NoError(t, result.Err)
		require.True(t, result.Success)
	})
}
