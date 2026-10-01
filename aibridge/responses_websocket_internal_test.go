package aibridge

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResponsesWebSocketURL requires that the upstream WebSocket URL keeps
// the provider base URL's host, path, and query, with its HTTP scheme
// switched to the matching WebSocket scheme. Integration tests only reach
// plain HTTP upstreams, but OpenAI's default base URL is HTTPS.
func TestResponsesWebSocketURL(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		baseURL string
		want    string
	}{
		{baseURL: "https://api.openai.com/v1/", want: "wss://api.openai.com/v1/responses"},
		{baseURL: "http://127.0.0.1:8080/v1", want: "ws://127.0.0.1:8080/v1/responses"},
		{baseURL: "https://gateway.example.com/openai/v1?api-version=1", want: "wss://gateway.example.com/openai/v1/responses?api-version=1"},
	} {
		got, err := responsesWebSocketURL(tc.baseURL)
		require.NoError(t, err, tc.baseURL)
		assert.Equal(t, tc.want, got, tc.baseURL)
	}

	_, err := responsesWebSocketURL("ftp://api.openai.com/v1/")
	require.Error(t, err)
}
