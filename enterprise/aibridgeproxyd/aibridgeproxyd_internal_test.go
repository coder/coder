package aibridgeproxyd

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
)

// TestReadErrorBodyForLog verifies that reading an aibridged error
// response body for logging leaves the body intact for downstream
// consumers (the proxy forwards it, and the response dumper reads it
// again), and that the logged rendering is capped.
func TestReadErrorBodyForLog(t *testing.T) {
	t.Parallel()

	newResponse := func(body string) *http.Response {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Body:       io.NopCloser(strings.NewReader(body)),
		}
	}

	t.Run("ReturnsBodyAndRestores", func(t *testing.T) {
		t.Parallel()
		s := &Server{ctx: t.Context(), logger: slogtest.Make(t, nil)}
		resp := newResponse(`{"error":"bad request"}`)

		got := s.readErrorBodyForLog(resp, s.logger)
		require.Equal(t, `{"error":"bad request"}`, got)

		// The body must still be readable in full for the proxy and the
		// response dumper.
		restored, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, `{"error":"bad request"}`, string(restored))
	})

	t.Run("TruncatesLargeBodyButRestoresFull", func(t *testing.T) {
		t.Parallel()
		s := &Server{ctx: t.Context(), logger: slogtest.Make(t, nil)}
		full := bytes.Repeat([]byte("a"), maxLoggedErrorBodyBytes+512)
		resp := newResponse(string(full))

		got := s.readErrorBodyForLog(resp, s.logger)
		require.Len(t, got, maxLoggedErrorBodyBytes+len("...(truncated)"))
		require.True(t, strings.HasSuffix(got, "...(truncated)"))

		// Truncation only affects the log string; the restored body is
		// the complete payload.
		restored, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, full, restored)
	})

	t.Run("NilBody", func(t *testing.T) {
		t.Parallel()
		s := &Server{ctx: t.Context(), logger: slogtest.Make(t, nil)}
		resp := &http.Response{StatusCode: http.StatusInternalServerError, Body: nil}

		require.Equal(t, "", s.readErrorBodyForLog(resp, s.logger))
	})
}
