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
	recorder         *httptest.ResponseRecorder
	clientConn       net.Conn
	serverConn       net.Conn
	serverReadWriter *bufio.ReadWriter
	close            func()
}

func (m mockWsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return m.serverConn, m.serverReadWriter, nil
}

func (m mockWsResponseWriter) Flush() {
	_ = m.serverReadWriter.Flush()
}

func (m mockWsResponseWriter) Header() http.Header {
	return m.recorder.Header()
}

func (m mockWsResponseWriter) Write(b []byte) (int, error) {
	return m.serverReadWriter.Write(b)
}

func (m mockWsResponseWriter) WriteHeader(code int) {
	m.recorder.WriteHeader(code)
}

type mockWsResponseWrite func(b []byte) (int, error)

func (w mockWsResponseWrite) Write(b []byte) (int, error) {
	return w(b)
}

func TestOneWayWebSocket(t *testing.T) {
	t.Parallel()

	newBaseRequest := func(t *testing.T) *http.Request {
		url := "ws://www.fake-website.com/logs"
		ctx := testutil.Context(t, testutil.WaitShort)
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
		server, client := net.Pipe()
		recorder := httptest.NewRecorder()

		var write mockWsResponseWrite = func(b []byte) (int, error) {
			serverCount, err := server.Write(b)
			if err != nil {
				return serverCount, err
			}
			recorderCount, err := recorder.Write(b)
			return min(serverCount, recorderCount), err
		}

		return mockWsResponseWriter{
			serverConn: server,
			clientConn: client,
			recorder:   recorder,
			serverReadWriter: bufio.NewReadWriter(
				bufio.NewReader(server),
				bufio.NewWriter(write),
			),
			close: func() {
				_ = recorder.Result().Body.Close()
				_ = server.Close()
				_ = client.Close()
			},
		}
	}

	t.Run("Produces an error if the socket connection could not be established", func(t *testing.T) {
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
			req := newBaseRequest(t)
			req.ProtoMajor = p.major
			req.ProtoMinor = p.minor
			req.Proto = p.proto

			writer := newWebsocketWriter()
			_, _, err := httpapi.OneWayWebSocket[any](writer, req)
			require.ErrorContains(t, err, p.proto)
			writer.close()
		}
	})

	t.Run("Returned callback can publish a new event to the WebSocket connection", func(t *testing.T) {
		t.Parallel()

		req := newBaseRequest(t)
		writer := newWebsocketWriter()
		defer writer.close()
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
		clientPayload := codersdk.ServerSentEvent{}
		err = json.Unmarshal(b, &clientPayload)
		require.NoError(t, err)
		require.Equal(t, serverPayload.Type, clientPayload.Type)
		cb, ok := clientPayload.Data.([]byte)
		require.True(t, ok)
		require.Equal(t, serverPayload.Data, string(cb))
	})

	t.Run("Signals to an outside consumer when the socket has been closed", func(t *testing.T) {
		t.Parallel()
	})

	t.Run("Socket will automatically close if client sends a single message", func(t *testing.T) {
		t.Parallel()
	})

	t.Run("Returned callback returns error if called after socket has been closed", func(t *testing.T) {
		t.Parallel()
	})

	t.Run("Sends a heartbeat to the socket on a fixed internal of time to keep connections alive", func(t *testing.T) {
		t.Parallel()
	})

	t.Run("Renders the socket inert if the request context cancels", func(t *testing.T) {
		t.Parallel()
	})
}
