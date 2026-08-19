package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

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
