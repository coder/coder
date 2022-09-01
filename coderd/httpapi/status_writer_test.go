package httpapi_test

import (
	"bufio"
	"crypto/rand"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"

	"github.com/coder/coder/coderd/httpapi"
)

func TestStatusWriter(t *testing.T) {
	t.Parallel()

	t.Run("WriteHeader", func(t *testing.T) {
		t.Parallel()

		var (
			rec = httptest.NewRecorder()
			w   = &httpapi.StatusWriter{ResponseWriter: rec}
		)

		w.WriteHeader(http.StatusOK)
		require.Equal(t, http.StatusOK, w.Status)
		// Validate that the code is written to the underlying Response.
		require.Equal(t, http.StatusOK, rec.Code)
	})

	t.Run("WriteHeaderTwice", func(t *testing.T) {
		t.Parallel()

		var (
			rec  = httptest.NewRecorder()
			w    = &httpapi.StatusWriter{ResponseWriter: rec}
			code = http.StatusNotFound
		)

		w.WriteHeader(code)
		w.WriteHeader(http.StatusOK)
		// Validate that we only record the first status code.
		require.Equal(t, code, w.Status)
		// Validate that the code is written to the underlying Response.
		require.Equal(t, code, rec.Code)
	})

	t.Run("WriteNoHeader", func(t *testing.T) {
		t.Parallel()
		var (
			rec  = httptest.NewRecorder()
			w    = &httpapi.StatusWriter{ResponseWriter: rec}
			body = []byte("hello")
		)

		_, err := w.Write(body)
		require.NoError(t, err)

		// Should set the status to OK.
		require.Equal(t, http.StatusOK, w.Status)
		// We don't record the body for codes <400.
		require.Equal(t, []byte(nil), w.ResponseBody())
		require.Equal(t, body, rec.Body.Bytes())
	})

	t.Run("WriteAfterHeader", func(t *testing.T) {
		t.Parallel()
		var (
			rec  = httptest.NewRecorder()
			w    = &httpapi.StatusWriter{ResponseWriter: rec}
			body = []byte("hello")
			code = http.StatusInternalServerError
		)

		w.WriteHeader(code)
		_, err := w.Write(body)
		require.NoError(t, err)

		require.Equal(t, code, w.Status)
		require.Equal(t, body, w.ResponseBody())
		require.Equal(t, body, rec.Body.Bytes())
	})

	t.Run("WriteMaxBody", func(t *testing.T) {
		t.Parallel()
		var (
			rec = httptest.NewRecorder()
			w   = &httpapi.StatusWriter{ResponseWriter: rec}
			// 8kb body.
			body = make([]byte, 8<<10)
			code = http.StatusInternalServerError
		)

		_, err := rand.Read(body)
		require.NoError(t, err)

		w.WriteHeader(code)
		_, err = w.Write(body)
		require.NoError(t, err)

		require.Equal(t, code, w.Status)
		require.Equal(t, body, rec.Body.Bytes())
		require.Equal(t, body[:4096], w.ResponseBody())
	})

	t.Run("Hijack", func(t *testing.T) {
		t.Parallel()
		var (
			rec = httptest.NewRecorder()
		)

		w := &httpapi.StatusWriter{ResponseWriter: hijacker{rec}}

		_, _, err := w.Hijack()
		require.Error(t, err)
		require.Equal(t, "hijacked", err.Error())
	})
}

type hijacker struct {
	http.ResponseWriter
}

func (hijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, xerrors.New("hijacked")
}
