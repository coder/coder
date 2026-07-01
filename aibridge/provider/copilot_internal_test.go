package provider

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
)

var testTracer = otel.Tracer("copilot_test")

func TestCopilot_TypeAndName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        config.Copilot
		expectType string
		expectName string
	}{
		{
			name:       "defaults",
			cfg:        config.Copilot{},
			expectType: config.ProviderCopilot,
			expectName: config.ProviderCopilot,
		},
		{
			name:       "custom_name",
			cfg:        config.Copilot{Name: "copilot-business"},
			expectType: config.ProviderCopilot,
			expectName: "copilot-business",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := NewCopilot(tc.cfg)
			assert.Equal(t, tc.expectType, p.Type())
			assert.Equal(t, tc.expectName, p.Name())
		})
	}
}

// TestCopilot_KeyFailoverConfig verifies that Copilot, being BYOK-only,
// returns a zero-value KeyFailoverConfig so that KeyFailoverTransport
// short-circuits and passes the request through unchanged.
func TestCopilot_KeyFailoverConfig(t *testing.T) {
	t.Parallel()

	p := NewCopilot(config.Copilot{})

	cfg := p.KeyFailoverConfig(slog.Make())

	assert.Equal(t, keypool.KeyFailoverConfig{}, cfg, "Copilot must return a zero-value KeyFailoverConfig to short-circuit the transport")
}

func TestCopilot_CreateInterceptor(t *testing.T) {
	t.Parallel()

	provider := NewCopilot(config.Copilot{})

	t.Run("MissingAuthorizationHeader", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "gpt-4.1", "messages": [{"role": "user", "content": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.Error(t, err)
		require.Nil(t, interceptor)
		assert.Contains(t, err.Error(), "missing Copilot authorization: Authorization header not found or invalid")
	})

	t.Run("InvalidAuthorizationFormat", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "InvalidFormat")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.Error(t, err)
		require.Nil(t, interceptor)
		assert.Contains(t, err.Error(), "missing Copilot authorization: Authorization header not found or invalid")
	})

	t.Run("ChatCompletions_NonStreamingRequest_BlockingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-haiku-4.5", "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.False(t, interceptor.Streaming())
	})

	t.Run("ChatCompletions_StreamingRequest_StreamingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "gpt-4.1", "messages": [{"role": "user", "content": "hello"}], "stream": true}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.True(t, interceptor.Streaming())
	})

	t.Run("ChatCompletions_InvalidRequestBody", func(t *testing.T) {
		t.Parallel()

		body := `invalid json`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.Error(t, err)
		require.Nil(t, interceptor)
		assert.Contains(t, err.Error(), "unmarshal chat completions request body")
	})

	t.Run("ChatCompletions_ClientHeaders", func(t *testing.T) {
		t.Parallel()

		var receivedHeaders http.Header

		// Mock upstream that captures headers
		mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"chatcmpl-123","object":"chat.completion","created":1677652288,"model":"gpt-4","choices":[{"index":0,"message":{"role":"assistant","content":"Hello!"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":12,"total_tokens":21}}`))
		}))
		t.Cleanup(mockUpstream.Close)

		// Create provider with mock upstream URL
		provider := NewCopilot(config.Copilot{
			BaseURL: mockUpstream.URL,
		})

		body := `{"model": "gpt-4", "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Editor-Version", "vscode/1.85.0")
		req.Header.Set("Copilot-Integration-Id", "test-integration")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)
		require.NoError(t, err)
		require.NotNil(t, interceptor)

		// Setup and process request
		logger := slog.Make()
		interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

		processReq := httptest.NewRequest(http.MethodPost, routeCopilotChatCompletions, nil)
		err = interceptor.ProcessRequest(w, processReq)
		require.NoError(t, err)

		// Verify Copilot-specific headers were forwarded.
		assert.Equal(t, "vscode/1.85.0", receivedHeaders.Get("Editor-Version"))
		assert.Equal(t, "test-integration", receivedHeaders.Get("Copilot-Integration-Id"))
		// Copilot uses per-user tokens: the client's Authorization must reach upstream as-is.
		assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"), "client Authorization must be used as provider key")
		assert.Empty(t, receivedHeaders.Get("X-Api-Key"), "X-Api-Key must not be set upstream")
	})

	t.Run("Responses_NonStreamingRequest_BlockingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "gpt-5-mini", "input": "hello", "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotResponses, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.False(t, interceptor.Streaming())
	})

	t.Run("Responses_StreamingRequest_StreamingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "gpt-5-mini", "input": "hello", "stream": true}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotResponses, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.True(t, interceptor.Streaming())
	})

	t.Run("Responses_InvalidRequestBody", func(t *testing.T) {
		t.Parallel()

		body := `invalid json`
		req := httptest.NewRequest(http.MethodPost, routeCopilotResponses, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.Error(t, err)
		require.Nil(t, interceptor)
		assert.Contains(t, err.Error(), "invalid JSON payload")
	})

	t.Run("Responses_ClientHeaders", func(t *testing.T) {
		t.Parallel()

		var receivedHeaders http.Header

		// Mock upstream that captures headers
		mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"resp-123","object":"responses.response","created":1677652288,"model":"gpt-5-mini","output":[],"usage":{"input_tokens":5,"output_tokens":10,"total_tokens":15}}`))
		}))
		t.Cleanup(mockUpstream.Close)

		// Create provider with mock upstream URL
		provider := NewCopilot(config.Copilot{
			BaseURL: mockUpstream.URL,
		})

		body := `{"model": "gpt-5-mini", "input": "hello", "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotResponses, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Editor-Version", "vscode/1.85.0")
		req.Header.Set("Copilot-Integration-Id", "test-integration")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)
		require.NoError(t, err)
		require.NotNil(t, interceptor)

		// Setup and process request
		logger := slog.Make()
		interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

		processReq := httptest.NewRequest(http.MethodPost, routeCopilotResponses, nil)
		err = interceptor.ProcessRequest(w, processReq)
		require.NoError(t, err)

		// Verify Copilot-specific headers were forwarded.
		assert.Equal(t, "vscode/1.85.0", receivedHeaders.Get("Editor-Version"))
		assert.Equal(t, "test-integration", receivedHeaders.Get("Copilot-Integration-Id"))
		// Copilot uses per-user tokens: the client's Authorization must reach upstream as-is.
		assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"), "client Authorization must be used as provider key")
		assert.Empty(t, receivedHeaders.Get("X-Api-Key"), "X-Api-Key must not be set upstream")
	})

	t.Run("Messages_NonStreamingRequest_BlockingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-sonnet-4.5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotMessages, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.False(t, interceptor.Streaming())
	})

	t.Run("Messages_StreamingRequest_StreamingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-sonnet-4.5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": true}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotMessages, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.True(t, interceptor.Streaming())
	})

	t.Run("Messages_InvalidRequestBody", func(t *testing.T) {
		t.Parallel()

		body := `invalid json`
		req := httptest.NewRequest(http.MethodPost, routeCopilotMessages, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.Error(t, err)
		require.Nil(t, interceptor)
		assert.Contains(t, err.Error(), "unmarshal request body")
	})

	t.Run("Messages_ClientHeaders", func(t *testing.T) {
		t.Parallel()

		var receivedHeaders http.Header

		// Mock upstream that captures headers.
		mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedHeaders = r.Header.Clone()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"id":"msg_123","type":"message","role":"assistant","model":"claude-sonnet-4.5","content":[{"type":"text","text":"Hello!"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":12}}`))
		}))
		t.Cleanup(mockUpstream.Close)

		// Create provider with mock upstream URL.
		provider := NewCopilot(config.Copilot{
			BaseURL: mockUpstream.URL,
		})

		body := `{"model": "claude-sonnet-4.5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeCopilotMessages, bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		req.Header.Set("Editor-Version", "vscode/1.85.0")
		req.Header.Set("Copilot-Integration-Id", "test-integration")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)
		require.NoError(t, err)
		require.NotNil(t, interceptor)

		// Setup and process request.
		logger := slog.Make()
		interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

		processReq := httptest.NewRequest(http.MethodPost, routeCopilotMessages, nil)
		err = interceptor.ProcessRequest(w, processReq)
		require.NoError(t, err)

		// Verify Copilot-specific headers were forwarded.
		assert.Equal(t, "vscode/1.85.0", receivedHeaders.Get("Editor-Version"))
		assert.Equal(t, "test-integration", receivedHeaders.Get("Copilot-Integration-Id"))
		// Copilot uses per-user tokens: the client's Authorization must reach upstream as-is.
		assert.Equal(t, "Bearer test-token", receivedHeaders.Get("Authorization"), "client Authorization must be used as provider key")
		assert.Empty(t, receivedHeaders.Get("X-Api-Key"), "X-Api-Key must not be set upstream")
	})

	t.Run("ErrUnknownRoute", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "gpt-4.1", "messages": [{"role": "user", "content": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/copilot/unknown/route", bytes.NewBufferString(body))
		req.Header.Set("Authorization", "Bearer test-token")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.ErrorIs(t, err, ErrUnknownRoute)
		require.Nil(t, interceptor)
	})
}
