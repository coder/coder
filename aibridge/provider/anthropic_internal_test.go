package provider

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/intercept"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/quartz"
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

func TestNewAnthropic_KeyResolution(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New([]string{"pool-key-0", "pool-key-1"}, quartz.NewMock(t))
	require.NoError(t, err)

	tests := []struct {
		name         string
		cfg          config.Anthropic
		expectedKeys []string
	}{
		{
			// Legacy single-key path: NewAnthropic builds a
			// pool containing just that key.
			name:         "key_creates_keypool",
			cfg:          config.Anthropic{Key: "legacy-key"},
			expectedKeys: []string{"legacy-key"},
		},
		{
			// Caller supplies the pool directly.
			name:         "keypool_passed_directly",
			cfg:          config.Anthropic{KeyPool: pool},
			expectedKeys: []string{"pool-key-0", "pool-key-1"},
		},
		{
			// Both set: KeyPool wins, Key is ignored.
			name:         "keypool_takes_precedence_over_key",
			cfg:          config.Anthropic{Key: "legacy-key", KeyPool: pool},
			expectedKeys: []string{"pool-key-0", "pool-key-1"},
		},
		{
			// Neither set: no centralized auth available. BYOK
			// auth is set per-request in CreateInterceptor.
			name:         "neither_set_no_centralized_auth",
			cfg:          config.Anthropic{},
			expectedKeys: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := NewAnthropic(tc.cfg, nil)

			if tc.expectedKeys == nil {
				assert.Nil(t, p.cfg.KeyPool, "expected no KeyPool")
				return
			}

			require.NotNil(t, p.cfg.KeyPool)
			walker := p.cfg.KeyPool.Walker()
			var got []string
			for {
				key, err := walker.Next()
				if err != nil {
					break
				}
				got = append(got, key.Value())
			}
			assert.Equal(t, tc.expectedKeys, got)
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

func TestAnthropic_KeyFailoverConfig(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New([]string{"k0", "k1"}, quartz.NewMock(t))
	require.NoError(t, err)

	p := NewAnthropic(config.Anthropic{KeyPool: pool}, nil)

	cfg := p.KeyFailoverConfig(slog.Make())

	assert.Same(t, pool, cfg.Pool, "Pool must be wired from the provider config")
	assert.Equal(t, config.ProviderAnthropic, cfg.ProviderName, "ProviderName must match the provider name")
	require.NotNil(t, cfg.IsBYOK)
	require.NotNil(t, cfg.InjectAuthKey)
	require.NotNil(t, cfg.BuildKeyPoolResponse)

	t.Run("IsBYOK", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name    string
			headers map[string]string
			want    bool
		}{
			{
				name:    "no_auth_headers",
				headers: nil,
				want:    false,
			},
			{
				name:    "x_api_key_only",
				headers: map[string]string{"X-Api-Key": "user-key"},
				want:    true,
			},
			{
				name:    "authorization_only",
				headers: map[string]string{"Authorization": "Bearer user-token"},
				want:    true,
			},
			{
				name: "both_headers_set",
				headers: map[string]string{
					"X-Api-Key":     "user-key",
					"Authorization": "Bearer user-token",
				},
				want: true,
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				r := httptest.NewRequest(http.MethodPost, "/", nil)
				for k, v := range tc.headers {
					r.Header.Set(k, v)
				}
				assert.Equal(t, tc.want, cfg.IsBYOK(r))
			})
		}
	})

	t.Run("InjectAuthKey", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			key  string
		}{
			{name: "writes_key_to_x_api_key", key: "centralized-key"},
			{name: "overwrites_existing_x_api_key", key: "next-key"},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				h := http.Header{"X-Api-Key": {"stale"}, "Authorization": {"Bearer stale"}}
				cfg.InjectAuthKey(&h, tc.key)
				assert.Equal(t, tc.key, h.Get("X-Api-Key"))
				assert.Equal(t, "Bearer stale", h.Get("Authorization"))
			})
		}
	})

	t.Run("BuildKeyPoolResponse", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name           string
			err            *keypool.Error
			wantStatus     int
			wantRetryAfter string
		}{
			{
				name:       "permanent_returns_502",
				err:        &keypool.Error{Kind: keypool.ErrorKindPermanent},
				wantStatus: http.StatusBadGateway,
			},
			{
				name:           "rate_limited_returns_429_with_retry_after",
				err:            &keypool.Error{Kind: keypool.ErrorKindRateLimited, RetryAfter: 5 * time.Second},
				wantStatus:     http.StatusTooManyRequests,
				wantRetryAfter: "5",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				resp := cfg.BuildKeyPoolResponse(tc.err)
				require.NotNil(t, resp)
				t.Cleanup(func() { _ = resp.Body.Close() })
				assert.Equal(t, tc.wantStatus, resp.StatusCode)
				assert.Equal(t, tc.wantRetryAfter, resp.Header.Get("Retry-After"))
			})
		}
	})
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
		{http.StatusTooManyRequests, false}, // 429: handled by key failover, not circuit breaker
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
