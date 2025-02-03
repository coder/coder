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
