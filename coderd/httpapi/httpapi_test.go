package httpapi_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
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

func TestOneWayWebSocket(t *testing.T) {
	t.Parallel()
	url := "ws://www.fake-website.com/logs"

	t.Run("Produces an error if the socket connection could not be established", func(t *testing.T) {
		t.Parallel()

		// WebSocket connections cannot be created on HTTP/1.0 and below
		ctx := testutil.Context(t, testutil.WaitShort)
		r, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		require.NoError(t, err)
		r.ProtoMajor = 1
		r.ProtoMinor = 0
		r.Proto = "HTTP/1.0"

		_, _, err = httpapi.OneWayWebSocket[any](httptest.NewRecorder(), r)
		require.ErrorContains(
			t,
			err,
			"WebSocket protocol violation: handshake request must be at least HTTP/1.1:",
		)
	})

	t.Run("Returned callback can publish a new event to the WebSocket connection", func(t *testing.T) {
		t.Parallel()
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
