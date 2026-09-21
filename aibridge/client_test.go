package aibridge_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/aibridge"
)

func TestGuessClientCompatibility(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://localhost", nil)
	require.NoError(t, err)
	req.Header.Set("User-Agent", "claude-cli/2.0")
	require.Equal(t, aibridge.ClientClaudeCode, aibridge.GuessClient(req))
}
