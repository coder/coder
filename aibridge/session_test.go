package aibridge_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
)

func TestGuessSessionIDCompatibility(t *testing.T) {
	t.Parallel()

	body := `{"metadata":{"user_id":"user_hash_session_session-id"}}`
	req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "http://localhost", strings.NewReader(body))
	require.NoError(t, err)
	require.Equal(t, new("session-id"), aibridge.GuessSessionID(aibridge.ClientClaudeCode, req))

	restored, err := io.ReadAll(req.Body)
	require.NoError(t, err)
	require.Equal(t, body, string(restored))
}
