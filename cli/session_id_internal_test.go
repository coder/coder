package cli

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"cdr.dev/slog/v3/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/serpent"
)

func TestResolveClientSessionID(t *testing.T) {
	t.Parallel()

	t.Run("GeneratesWhenUnset", func(t *testing.T) {
		t.Parallel()

		inv := &serpent.Invocation{Stderr: io.Discard}
		id, err := resolveClientSessionID(inv)
		require.NoError(t, err)
		require.True(t, tracing.ValidSessionID(id), "generated session ID must be valid")
	})

	t.Run("UsesValidEnv", func(t *testing.T) {
		t.Parallel()

		const want = "0123456789abcdef0123456789abcdef"
		inv := &serpent.Invocation{Stderr: io.Discard}
		inv.Environ.Set(clientSessionIDEnv, want)
		id, err := resolveClientSessionID(inv)
		require.NoError(t, err)
		require.Equal(t, want, id)
	})

	t.Run("UsesMalformedEnvVerbatim", func(t *testing.T) {
		t.Parallel()

		const want = "not-a-valid-session-id"
		inv := &serpent.Invocation{Stderr: io.Discard}
		inv.Environ.Set(clientSessionIDEnv, want)
		id, err := resolveClientSessionID(inv)
		require.NoError(t, err)
		require.Equal(t, want, id, "a set CODER_TRACE_SESSION_ID is used verbatim")
	})

	t.Run("GeneratesWhenEnvEmpty", func(t *testing.T) {
		t.Parallel()

		inv := &serpent.Invocation{Stderr: io.Discard}
		inv.Environ.Set(clientSessionIDEnv, "")
		id, err := resolveClientSessionID(inv)
		require.NoError(t, err)
		require.True(t, tracing.ValidSessionID(id), "empty env must fall back to a generated ID")
	})
}

func TestClientSessionIDMiddleware(t *testing.T) {
	t.Parallel()

	// runMiddleware runs clientSessionIDMiddleware around a handler that
	// captures the resulting invocation, returning it for assertions.
	runMiddleware := func(t *testing.T, inv *serpent.Invocation) *serpent.Invocation {
		t.Helper()
		var got *serpent.Invocation
		handler := clientSessionIDMiddleware()(func(i *serpent.Invocation) error {
			got = i
			return nil
		})
		require.NoError(t, handler(inv))
		require.NotNil(t, got)
		return got
	}

	t.Run("GeneratesAndStoresOnContext", func(t *testing.T) {
		t.Parallel()

		inv := (&serpent.Invocation{Stderr: io.Discard}).WithContext(t.Context())
		got := runMiddleware(t, inv)
		id := clientSessionIDFromContext(got.Context())
		require.True(t, tracing.ValidSessionID(id), "middleware must store a valid generated ID")
	})

	t.Run("UsesEnv", func(t *testing.T) {
		t.Parallel()

		const want = "0123456789abcdef0123456789abcdef"
		inv := (&serpent.Invocation{Stderr: io.Discard}).WithContext(t.Context())
		inv.Environ.Set(clientSessionIDEnv, want)
		got := runMiddleware(t, inv)
		require.Equal(t, want, clientSessionIDFromContext(got.Context()))
	})

	t.Run("AttachesSlogField", func(t *testing.T) {
		t.Parallel()

		inv := (&serpent.Invocation{Stderr: io.Discard}).WithContext(t.Context())
		got := runMiddleware(t, inv)
		id := clientSessionIDFromContext(got.Context())
		require.NotEmpty(t, id)

		// A fresh logger that logs with the invocation context must include the
		// client_session_id field, proving the field rides on the context rather
		// than a specific logger instance.
		var buf bytes.Buffer
		logger := slog.Make(sloghuman.Sink(&buf))
		logger.Info(got.Context(), "session id log line")
		require.Contains(t, buf.String(), "client_session_id="+id)
	})

	t.Run("SkipsCompletionMode", func(t *testing.T) {
		t.Parallel()

		inv := (&serpent.Invocation{Stderr: io.Discard}).
			WithContext(t.Context())
		inv.Environ.Set(serpent.CompletionModeEnv, "1")
		got := runMiddleware(t, inv)
		require.Empty(t, clientSessionIDFromContext(got.Context()),
			"completion mode must not resolve a session ID")
	})
}

func TestWrapTransportWithSessionIDHeader(t *testing.T) {
	t.Parallel()

	const sessionID = "0123456789abcdef0123456789abcdef"

	var gotHeader http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		rw.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := &http.Client{
		Transport: wrapTransportWithSessionIDHeader(http.DefaultTransport, sessionID),
	}

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	// The baggage header the server receives must carry the session ID under
	// the client_session_id key so the tracing middleware can extract it.
	require.Equal(t, tracing.SessionIDBaggageKey+"="+sessionID, gotHeader.Get("baggage"))
}
