package aibridge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3/sloggers/slogtest"
	"github.com/coder/coder/v2/aibridge/config"
	aibcontext "github.com/coder/coder/v2/aibridge/context"
	aibheaders "github.com/coder/coder/v2/aibridge/headers"
	"github.com/coder/coder/v2/aibridge/provider"
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

// TestWriteHTTPResponseRetryAfter requires that a refused WebSocket client
// gets upstream's retry hint as Retry-After, including a hint upstream gave
// only in OpenAI's retry-after-ms header, as the HTTP path does.
func TestWriteHTTPResponseRetryAfter(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name   string
		header http.Header
		want   string
	}{
		{name: "Seconds", header: http.Header{"Retry-After": {"7"}}, want: "7"},
		{name: "Milliseconds", header: http.Header{"Retry-After-Ms": {"1500"}}, want: "2"},
		{name: "Both", header: http.Header{"Retry-After": {"7"}, "Retry-After-Ms": {"1500"}}, want: "7"},
		{name: "None", header: http.Header{}, want: ""},
	} {
		rec := httptest.NewRecorder()
		writeHTTPResponse(rec, &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     tc.header,
			Body:       io.NopCloser(strings.NewReader(`{"error":{}}`)),
		})
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, tc.name)
		assert.Equal(t, tc.want, rec.Header().Get("Retry-After"), tc.name)
	}
}

// TestResponsesWebSocketUpstreamHeadersKeepActorHeaders requires that an
// actor header configured at a name the WebSocket handshake strips from the
// client still reaches upstream with the actor's value, as it does over
// HTTP, while the client's own value never does.
func TestResponsesWebSocketUpstreamHeadersKeepActorHeaders(t *testing.T) {
	t.Parallel()

	prov := provider.NewOpenAI(config.OpenAI{ActorHeaderNames: map[string]string{aibheaders.ActorAttributeID: "Origin"}})
	h := newResponsesWebSocketHandler(prov, nil, slogtest.Make(t, nil))
	r := httptest.NewRequest(http.MethodGet, "/openai/v1/responses", nil)
	r.Header.Set("Origin", "https://client.example")
	r.Header.Set("Sec-WebSocket-Key", "client-key")
	r.Header.Set("Cookie", "session=1")

	got := h.upstreamHeaders(r, &aibcontext.Actor{ID: "user-1"}, "sk-test")
	assert.Equal(t, "user-1", got.Get("Origin"))
	assert.Empty(t, got.Get("Sec-WebSocket-Key"))
	assert.Empty(t, got.Get("Cookie"))
	assert.Equal(t, "Bearer sk-test", got.Get("Authorization"))
}
