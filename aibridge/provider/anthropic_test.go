package provider //nolint:testpackage // tests unexported internals

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"

	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/internal/testutil"
)

func TestAnthropic_TypeAndName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		cfg        config.Anthropic
		expectType string
		expectName string
	}{
		{
			name:       "defaults",
			cfg:        config.Anthropic{},
			expectType: config.ProviderAnthropic,
			expectName: config.ProviderAnthropic,
		},
		{
			name:       "custom_name",
			cfg:        config.Anthropic{Name: "anthropic-custom"},
			expectType: config.ProviderAnthropic,
			expectName: "anthropic-custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			p := NewAnthropic(tc.cfg, nil)
			assert.Equal(t, tc.expectType, p.Type())
			assert.Equal(t, tc.expectName, p.Name())
		})
	}
}

func TestAnthropic_CreateInterceptor(t *testing.T) {
	t.Parallel()

	provider := NewAnthropic(config.Anthropic{Key: "test-key"}, nil)

	t.Run("Messages_NonStreamingRequest_BlockingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.False(t, interceptor.Streaming())
	})

	t.Run("Messages_StreamingRequest_StreamingInterceptor", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": true}`
		req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.NoError(t, err)
		require.NotNil(t, interceptor)
		assert.True(t, interceptor.Streaming())
	})

	t.Run("Messages_InvalidRequestBody", func(t *testing.T) {
		t.Parallel()

		body := `invalid json`
		req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
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
			_, _ = w.Write([]byte(`{"id":"msg-123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"claude-opus-4-5","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
		}))
		t.Cleanup(mockUpstream.Close)

		provider := NewAnthropic(config.Anthropic{
			BaseURL: mockUpstream.URL,
			Key:     "test-key",
		}, nil)

		// Use a realistic multi-beta value as sent by Claude Code clients.
		betaHeader := "claude-code-20250219,adaptive-thinking-2026-01-28,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24"

		body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
		req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
		req.Header.Set("Anthropic-Beta", betaHeader)
		// Simulate a client sending both Authorization and X-Api-Key headers.
		// In this case, only the X-Api-Key header is preserved.
		req.Header.Set("Authorization", "Bearer fake-client-bearer")
		req.Header.Set("X-Api-Key", "personal user key")
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)
		require.NoError(t, err)
		require.NotNil(t, interceptor)

		logger := slog.Make()
		interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

		processReq := httptest.NewRequest(http.MethodPost, routeMessages, nil)
		err = interceptor.ProcessRequest(w, processReq)
		require.NoError(t, err)

		// Verify the full Anthropic-Beta header (all betas) was forwarded unchanged.
		assert.Equal(t, betaHeader, receivedHeaders.Get("Anthropic-Beta"), "Anthropic-Beta header must be forwarded unchanged to upstream")

		// Verify user's personal key was used and the authorization header was not forwarded.
		assert.Equal(t, "personal user key", receivedHeaders.Get("X-Api-Key"), "upstream must receive personal user key")
		assert.Empty(t, receivedHeaders.Get("Authorization"), "client Authorization header must not reach upstream")
	})

	t.Run("ErrUnknownRoute", func(t *testing.T) {
		t.Parallel()

		body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}]}`
		req := httptest.NewRequest(http.MethodPost, "/anthropic/unknown/route", bytes.NewBufferString(body))
		w := httptest.NewRecorder()

		interceptor, err := provider.CreateInterceptor(w, req, testTracer)

		require.ErrorIs(t, err, ErrUnknownRoute)
		require.Nil(t, interceptor)
	})
}

func TestAnthropic_CreateInterceptor_BYOK(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		setHeaders         map[string]string
		wantXApiKey        string
		wantAuthorization  string
		wantCredentialKind intercept.CredentialKind
		wantCredentialHint string
	}{
		{
			name:               "Messages_BYOK_BearerToken",
			setHeaders:         map[string]string{"Authorization": "Bearer user-access-token"},
			wantAuthorization:  "Bearer user-access-token",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
		},
		{
			name:               "Messages_BYOK_APIKey",
			setHeaders:         map[string]string{"X-Api-Key": "user-api-key"},
			wantXApiKey:        "user-api-key",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...ey",
		},
		{
			name:               "Messages_Centralized",
			setHeaders:         map[string]string{},
			wantXApiKey:        "test-key",
			wantCredentialKind: intercept.CredentialKindCentralized,
			wantCredentialHint: "t...y",
		},
		{
			name: "Messages_BYOK_BearerToken_And_APIKey",
			setHeaders: map[string]string{
				"Authorization": "Bearer user-access-token",
				"X-Api-Key":     "user-api-key",
			},
			wantXApiKey:        "user-api-key",
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...ey",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			var receivedHeaders http.Header

			mockUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				receivedHeaders = r.Header.Clone()
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"id":"msg-123","type":"message","role":"assistant","content":[{"type":"text","text":"Hello!"}],"model":"claude-opus-4-5","stop_reason":"end_turn","usage":{"input_tokens":10,"output_tokens":5}}`))
			}))
			t.Cleanup(mockUpstream.Close)

			provider := NewAnthropic(config.Anthropic{
				BaseURL: mockUpstream.URL,
				Key:     "test-key",
			}, nil)

			body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
			req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
			for k, v := range tc.setHeaders {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			interceptor, err := provider.CreateInterceptor(w, req, testTracer)
			require.NoError(t, err)
			require.NotNil(t, interceptor)

			cred := interceptor.Credential()
			assert.Equal(t, tc.wantCredentialKind, cred.Kind, "credential kind mismatch")
			assert.Equal(t, tc.wantCredentialHint, cred.Hint, "credential hint mismatch")

			logger := slog.Make()
			interceptor.Setup(logger, &testutil.MockRecorder{}, nil)

			processReq := httptest.NewRequest(http.MethodPost, routeMessages, nil)
			err = interceptor.ProcessRequest(w, processReq)
			require.NoError(t, err)

			assert.Equal(t, tc.wantXApiKey, receivedHeaders.Get("X-Api-Key"))
			assert.Equal(t, tc.wantAuthorization, receivedHeaders.Get("Authorization"))
		})
	}
}

func TestAnthropic_InjectAuthHeader(t *testing.T) {
	t.Parallel()

	provider := NewAnthropic(config.Anthropic{Key: "centralized-key"}, nil)

	tests := []struct {
		name              string
		presetHeaders     map[string]string
		wantXApiKey       string
		wantAuthorization string
	}{
		{
			name:          "when no auth headers are provided, inject centralized key",
			presetHeaders: map[string]string{},
			wantXApiKey:   "centralized-key",
		},
		{
			name:          "when X-Api-Key header is provided, use it",
			presetHeaders: map[string]string{"X-Api-Key": "user-api-key"},
			wantXApiKey:   "user-api-key",
		},
		{
			name:              "when Authorization header is provided, use it",
			presetHeaders:     map[string]string{"Authorization": "Bearer user-access-token"},
			wantAuthorization: "Bearer user-access-token",
		},
		{
			name: "when both headers are provided, keep both",
			presetHeaders: map[string]string{
				"Authorization": "Bearer user-access-token",
				"X-Api-Key":     "user-api-key",
			},
			wantXApiKey:       "user-api-key",
			wantAuthorization: "Bearer user-access-token",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			headers := http.Header{}
			for k, v := range tc.presetHeaders {
				headers.Set(k, v)
			}

			provider.InjectAuthHeader(&headers)

			assert.Equal(t, tc.wantXApiKey, headers.Get("X-Api-Key"))
			assert.Equal(t, tc.wantAuthorization, headers.Get("Authorization"))
		})
	}
}

func TestExtractAnthropicHeaders(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		headers  map[string]string
		expected map[string]string
	}{
		{
			name:     "no headers",
			headers:  map[string]string{},
			expected: map[string]string{},
		},
		{
			name:     "single beta",
			headers:  map[string]string{"Anthropic-Beta": "claude-code-20250219"},
			expected: map[string]string{"Anthropic-Beta": "claude-code-20250219"},
		},
		{
			name:     "multiple betas in single header",
			headers:  map[string]string{"Anthropic-Beta": "claude-code-20250219,adaptive-thinking-2026-01-28,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24"},
			expected: map[string]string{"Anthropic-Beta": "claude-code-20250219,adaptive-thinking-2026-01-28,context-management-2025-06-27,prompt-caching-scope-2026-01-05,effort-2025-11-24"},
		},
		{
			name:     "ignores other headers",
			headers:  map[string]string{"Anthropic-Beta": "claude-code-20250219,context-management-2025-06-27", "X-Api-Key": "secret"},
			expected: map[string]string{"Anthropic-Beta": "claude-code-20250219,context-management-2025-06-27"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/", nil)
			for header, value := range tc.headers {
				req.Header.Set(header, value)
			}

			result := extractAnthropicHeaders(req)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func Test_anthropicIsFailure(t *testing.T) {
	t.Parallel()

	tests := []struct {
		statusCode int
		isFailure  bool
	}{
		{http.StatusOK, false},
		{http.StatusBadRequest, false},
		{http.StatusUnauthorized, false},
		{http.StatusTooManyRequests, true}, // 429
		{http.StatusInternalServerError, false},
		{http.StatusBadGateway, false},
		{http.StatusServiceUnavailable, true}, // 503
		{http.StatusGatewayTimeout, true},     // 504
		{529, true},                           // Anthropic Overloaded
	}

	for _, tt := range tests {
		assert.Equal(t, tt.isFailure, anthropicIsFailure(tt.statusCode), "status code %d", tt.statusCode)
	}
}
