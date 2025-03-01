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
// but it must also implement http.Hijack
type mockWsResponseWriter struct {
	serverRecorder   *httptest.ResponseRecorder
	serverConn       net.Conn
	clientConn       net.Conn
	serverReadWriter *bufio.ReadWriter
}

func (m mockWsResponseWriter) Close() {
	_ = m.serverRecorder.Result().Body.Close()
	_ = m.serverConn.Close()
	_ = m.clientConn.Close()
}

func (m mockWsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.serverReadWriter, nil
}

func (m mockWsResponseWriter) Flush() {
	_ = m.serverReadWriter.Flush()
}

func (m mockWsResponseWriter) Header() http.Header {
	return m.serverRecorder.Header()
}

func (m mockWsResponseWriter) Write(b []byte) (int, error) {
	return m.serverReadWriter.Write(b)
}

func (m mockWsResponseWriter) WriteHeader(code int) {
	m.serverRecorder.WriteHeader(code)
}

type mockWsWrite func(b []byte) (int, error)

func (w mockWsWrite) Write(b []byte) (int, error) {
	return w(b)
}

func TestOneWayWebSocket(t *testing.T) {
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

	newWebsocketWriter := func() mockWsResponseWriter {
		mockServer, mockClient := net.Pipe()
		recorder := httptest.NewRecorder()

		var write mockWsWrite = func(b []byte) (int, error) {
			serverCount, err := mockServer.Write(b)
			if err != nil {
				return serverCount, err
			}
			recorderCount, err := recorder.Write(b)
			return min(serverCount, recorderCount), err
		}

		return mockWsResponseWriter{
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

			writer := newWebsocketWriter()
			t.Cleanup(writer.Close)
			_, _, err := httpapi.OneWayWebSocket[any](writer, req)
			require.ErrorContains(t, err, p.proto)
		}
	})

	t.Run("Returned callback can publish new event to WebSocket connection", func(t *testing.T) {
		t.Parallel()

		ctx := testutil.Context(t, testutil.WaitShort)
		req := newBaseRequest(ctx)
		writer := newWebsocketWriter()
		t.Cleanup(writer.Close)
		send, _, err := httpapi.OneWayWebSocket[codersdk.ServerSentEvent](writer, req)
		require.NoError(t, err)

		serverPayload := codersdk.ServerSentEvent{
			Type: codersdk.ServerSentEventTypeData,
			Data: "Blah",
		}
		err = send(serverPayload)
		require.NoError(t, err)

		b, err := io.ReadAll(writer.clientConn)
		require.NoError(t, err)
		fmt.Printf("-----------%q\n", b) // todo: Figure out why junk characters are added to JSON
		clientPayload := codersdk.ServerSentEvent{}
		err = json.Unmarshal(b, &clientPayload)
		require.NoError(t, err)
		require.Equal(t, serverPayload.Type, clientPayload.Type)
		data, ok := clientPayload.Data.([]byte)
		require.True(t, ok)
		require.Equal(t, serverPayload.Data, string(data))
	})

	t.Run("Signals to outside consumer when socket has been closed", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(testutil.Context(t, testutil.WaitShort))
		req := newBaseRequest(ctx)
		writer := newWebsocketWriter()
		t.Cleanup(writer.Close)
		_, done, err := httpapi.OneWayWebSocket[codersdk.ServerSentEvent](writer, req)
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
		writer := newWebsocketWriter()
		t.Cleanup(writer.Close)
		_, done, err := httpapi.OneWayWebSocket[codersdk.ServerSentEvent](writer, req)
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
		writer := newWebsocketWriter()
		t.Cleanup(writer.Close)
		send, done, err := httpapi.OneWayWebSocket[codersdk.ServerSentEvent](writer, req)
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

		timeout := httpapi.HeartbeatInterval + (5 * time.Second)
		ctx := testutil.Context(t, timeout)
		req := newBaseRequest(ctx)
		writer := newWebsocketWriter()
		t.Cleanup(writer.Close)
		_, _, err := httpapi.OneWayWebSocket[codersdk.ServerSentEvent](writer, req)
		require.NoError(t, err)

		type Result struct {
			Err     error
			Success bool
		}
		resultC := make(chan Result)
		go func() {
			err := writer.
				clientConn.
				SetReadDeadline(time.Now().Add(httpapi.HeartbeatInterval))
			if err != nil {
				resultC <- Result{err, false}
				return
			}
			pingBuffer := make([]byte, 1)
			pingSize, err := writer.clientConn.Read(pingBuffer)
			resultC <- Result{err, pingSize == 1}
		}()

		result := <-resultC
		require.NoError(t, result.Err)
		require.True(t, result.Success)
	})
}
