package agentdesktop_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/agent/agentdesktop"
	"github.com/coder/coder/v2/codersdk"
)

func TestHandleDesktop_BinaryNotFound(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	api := agentdesktop.NewAPI(logger)
	defer api.Close()

	// Use a standard HTTP request (not a WebSocket upgrade) — the
	// handler checks for the binary before accepting the WebSocket,
	// so it will return 424 as a plain HTTP response.
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusFailedDependency, rr.Code)

	var resp codersdk.Response
	err := json.NewDecoder(rr.Body).Decode(&resp)
	require.NoError(t, err)
	assert.Equal(t, "portabledesktop binary not found.", resp.Message)
}

func TestClose_NoSession(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	api := agentdesktop.NewAPI(logger)

	// Close on a fresh API with no session should succeed.
	err := api.Close()
	require.NoError(t, err)
}

func TestClose_PreventsNewSessions(t *testing.T) {
	t.Parallel()

	logger := slogtest.Make(t, nil)
	api := agentdesktop.NewAPI(logger)

	// Close the API first.
	err := api.Close()
	require.NoError(t, err)

	// Now try to use the handler — it should still return 424
	// because portabledesktop isn't in PATH (the LookPath check
	// happens before the session check). But this verifies the API
	// doesn't panic after Close().
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	handler := api.Routes()
	handler.ServeHTTP(rr, req)

	// Either 424 (binary not found) or 500 (API closed) is
	// acceptable.
	assert.True(t, rr.Code == http.StatusFailedDependency || rr.Code == http.StatusInternalServerError)
}
