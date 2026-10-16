package provider

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
)

// TestOpenAI_UpstreamHeaders asserts admin-configured custom upstream headers
// reach the upstream on the wire: absent by default, present when configured,
// with per-conversation placeholder resolution. It drives the full
// CreateInterceptor plus ProcessRequest path against a capturing mock
// upstream, so it covers the middleware the bridged routes install.
func TestOpenAI_UpstreamHeaders(t *testing.T) {
	t.Parallel()

	const chatBody = `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "stream": false}`
	const responsesBody = `{"model": "gpt-5", "input": [{"type": "message", "role": "user", "content": "hello"}], "stream": false}`

	tests := []struct {
		name string
		// route and request/response bodies select the bridged route under test.
		route         string
		requestBody   string
		responseBody  string
		upstream      map[string]string
		clientHeaders map[string]string
		// wantHeaders maps upstream header name to the exact expected value.
		// A value of wantStableFallback asserts a non-empty value that is
		// identical across two consecutive requests instead.
		wantHeaders map[string]string
	}{
		{
			name:          "AbsentByDefault",
			route:         routeChatCompletions,
			requestBody:   chatBody,
			responseBody:  chatCompletionResponse,
			upstream:      nil,
			clientHeaders: map[string]string{"X-Coder-Chat-Id": "chat-abc"},
			wantHeaders:   map[string]string{"X-Opencode-Session": ""},
		},
		{
			name:          "LiteralHeaderOnTheWire",
			route:         routeChatCompletions,
			requestBody:   chatBody,
			responseBody:  chatCompletionResponse,
			upstream:      map[string]string{"x-opencode-session": "session-123"},
			clientHeaders: map[string]string{},
			wantHeaders:   map[string]string{"X-Opencode-Session": "session-123"},
		},
		{
			name:         "MultipleHeaders",
			route:        routeChatCompletions,
			requestBody:  chatBody,
			responseBody: chatCompletionResponse,
			upstream: map[string]string{
				"x-opencode-session": "session-123",
				"X-Custom-Two":       "second",
			},
			clientHeaders: map[string]string{},
			wantHeaders: map[string]string{
				"X-Opencode-Session": "session-123",
				"X-Custom-Two":       "second",
			},
		},
		{
			name:          "PlaceholderResolvesConversationID",
			route:         routeChatCompletions,
			requestBody:   chatBody,
			responseBody:  chatCompletionResponse,
			upstream:      map[string]string{"x-opencode-session": "{{chat_id}}"},
			clientHeaders: map[string]string{"X-Coder-Chat-Id": "chat-abc"},
			wantHeaders:   map[string]string{"X-Opencode-Session": "chat-abc"},
		},
		{
			name:          "PlaceholderFallsBackToStableValue",
			route:         routeChatCompletions,
			requestBody:   chatBody,
			responseBody:  chatCompletionResponse,
			upstream:      map[string]string{"x-opencode-session": "{{chat_id}}"},
			clientHeaders: map[string]string{},
			wantHeaders:   map[string]string{"X-Opencode-Session": wantStableFallback},
		},
		{
			name:          "ResponsesRouteAppliesHeaders",
			route:         routeResponses,
			requestBody:   responsesBody,
			responseBody:  responsesAPIResponse,
			upstream:      map[string]string{"x-opencode-session": "session-123"},
			clientHeaders: map[string]string{},
			wantHeaders:   map[string]string{"X-Opencode-Session": "session-123"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var received []http.Header
			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				received = append(received, r.Header.Clone())
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, err := w.Write([]byte(tc.responseBody))
				require.NoError(t, err)
			}))
			t.Cleanup(mockUpstream.Close)

			provider := NewOpenAI(config.OpenAI{
				BaseURL:         mockUpstream.URL,
				KeyPool:         testutil.SingleKeyPool(config.ProviderOpenAI, "centralized-key"),
				UpstreamHeaders: tc.upstream,
			})

			newRequest := func() *http.Request {
				req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, provider.RoutePrefix()+tc.route, bytes.NewBufferString(tc.requestBody))
				for k, v := range tc.clientHeaders {
					req.Header.Set(k, v)
				}
				return req
			}

			// Run the request twice so the stable-fallback case can assert
			// both requests carried the same session value.
			for range 2 {
				w := httptest.NewRecorder()
				interceptor, err := provider.CreateInterceptor(w, newRequest(), testTracer)
				require.NoError(t, err)
				require.NotNil(t, interceptor)
				interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)
				processReq := httptest.NewRequestWithContext(context.Background(), http.MethodPost, provider.RoutePrefix()+tc.route, nil)
				require.NoError(t, interceptor.ProcessRequest(w, processReq))
			}
			require.Len(t, received, 2)

			for name, want := range tc.wantHeaders {
				if want == wantStableFallback {
					got := received[0].Get(name)
					require.NotEmpty(t, got, "fallback session header must be set")
					require.NoError(t, uuid.Validate(got), "fallback session header must be a UUID")
					assert.Equal(t, got, received[1].Get(name), "fallback session value must be stable across requests")
					continue
				}
				assert.Equal(t, want, received[0].Get(name), "upstream header %q", name)
			}
		})
	}
}

// wantStableFallback marks an expectation of a stable generated fallback
// value rather than a literal string. See TestOpenAI_UpstreamHeaders.
const wantStableFallback = "\x00stable-fallback"
