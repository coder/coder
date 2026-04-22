package provider //nolint:testpackage // tests unexported internals

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

func TestCopilot_InjectAuthHeader(t *testing.T) {
	t.Parallel()

	// Copilot uses per-user key passed in the Authorization header,
	// so InjectAuthHeader should not modify any headers.
	provider := NewCopilot(config.Copilot{})

	t.Run("ExistingHeaders_Unchanged", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{}
		headers.Set("Authorization", "Bearer user-token")
		headers.Set("X-Custom-Header", "custom-value")

		provider.InjectAuthHeader(&headers)

		assert.Equal(t, "Bearer user-token", headers.Get("Authorization"),
			"Authorization header should remain unchanged")
		assert.Equal(t, "custom-value", headers.Get("X-Custom-Header"),
			"other headers should remain unchanged")
	})

	t.Run("EmptyHeaders_NoneAdded", func(t *testing.T) {
		t.Parallel()

		headers := http.Header{}

		provider.InjectAuthHeader(&headers)

		assert.Empty(t, headers, "no headers should be added")
	})
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

func TestExtractCopilotHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		expected map[string]string
	}{
		{
			name:     "all headers present",
			headers:  map[string]string{"Editor-Version": "vscode/1.85.0", "Copilot-Integration-Id": "some-id"},
			expected: map[string]string{"Editor-Version": "vscode/1.85.0", "Copilot-Integration-Id": "some-id"},
		},
		{
			name:     "some headers present",
			headers:  map[string]string{"Editor-Version": "vscode/1.85.0"},
			expected: map[string]string{"Editor-Version": "vscode/1.85.0"},
		},
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: map[string]string{},
		},
		{
			name:     "ignores other headers",
			headers:  map[string]string{"Editor-Version": "vscode/1.85.0", "Authorization": "Bearer token"},
			expected: map[string]string{"Editor-Version": "vscode/1.85.0"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			for header, value := range tc.headers {
				req.Header.Set(header, value)
			}

			result := extractCopilotHeaders(req)
			assert.Equal(t, tc.expected, result)
		})
	}
}
