package provider

import (
	"bytes"
	"context"
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

// newTestAnthropic is local (not aibridgetest.NewAnthropicProvider) because these
// white-box tests need the concrete *Anthropic, and importing aibridgetest here
// would create an import cycle.
func newTestAnthropic(t testing.TB, cfg config.Anthropic, bedrockCfg *config.AWSBedrock) *Anthropic {
	t.Helper()
	p, err := NewAnthropic(context.Background(), cfg, bedrockCfg)
	require.NoError(t, err)
	return p
}

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

			p := newTestAnthropic(t, tc.cfg, nil)
			assert.Equal(t, tc.expectType, p.Type())
			assert.Equal(t, tc.expectName, p.Name())
		})
	}
}

func TestNewAnthropic_KeyResolution(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderAnthropic, []string{"pool-key-0", "pool-key-1"}, quartz.NewMock(t), nil)
	require.NoError(t, err)

	tests := []struct {
		name         string
		cfg          config.Anthropic
		expectedKeys []string
	}{
		{
			// Caller supplies the pool directly.
			name:         "keypool_passed_directly",
			cfg:          config.Anthropic{KeyPool: pool},
			expectedKeys: []string{"pool-key-0", "pool-key-1"},
		},
		{
			// No pool: no centralized auth available. BYOK auth is
			// resolved per-request in CreateInterceptor.
			name:         "no_keypool_no_centralized_auth",
			cfg:          config.Anthropic{},
			expectedKeys: nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := newTestAnthropic(t, tc.cfg, nil)

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

// NOTE: no t.Parallel() because the subtests use t.Setenv.
func TestNewAnthropic_BedrockRegionResolution(t *testing.T) {
	t.Run("mantle_region_from_env", func(t *testing.T) {
		t.Setenv("AWS_REGION", "us-west-2")

		p, err := NewAnthropic(context.Background(), config.Anthropic{}, &config.AWSBedrock{
			BaseURL:         "https://bedrock-mantle.us-west-2.api.aws/anthropic",
			Protocol:        config.BedrockProtocolMantle,
			AccessKey:       "test-key",
			AccessKeySecret: "test-secret",
		})
		require.NoError(t, err)
		require.NotNil(t, p.bedrock)
		require.Equal(t, "us-west-2", p.bedrock.Cfg.Region)
	})

	t.Run("mantle_no_region_anywhere", func(t *testing.T) {
		// Clear every source the AWS SDK consults for a region so none
		// resolves, then confirm construction rejects the mantle provider.
		t.Setenv("AWS_REGION", "")
		t.Setenv("AWS_DEFAULT_REGION", "")
		t.Setenv("AWS_PROFILE", "")
		t.Setenv("AWS_CONFIG_FILE", "/dev/null")
		t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/dev/null")
		t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

		_, err := NewAnthropic(context.Background(), config.Anthropic{}, &config.AWSBedrock{
			BaseURL:         "https://proxy.internal",
			Protocol:        config.BedrockProtocolMantle,
			AccessKey:       "test-key",
			AccessKeySecret: "test-secret",
		})
		require.ErrorContains(t, err, "region required")
	})
}

func TestAnthropic_CreateInterceptor(t *testing.T) {
	t.Parallel()

	provider := newTestAnthropic(t, config.Anthropic{KeyPool: testutil.SingleKeyPool(config.ProviderAnthropic, "test-key")}, nil)

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

		provider := newTestAnthropic(t, config.Anthropic{
			BaseURL: mockUpstream.URL,
			KeyPool: testutil.SingleKeyPool(config.ProviderAnthropic, "test-key"),
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

func TestAnthropic_CreateInterceptor_Credential(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pool    bool // provider has a centralized "test-key" pool
		bedrock bool // Bedrock-backed provider (authenticates via AWS signing)
		// bedrockStatic, when bedrock is set, configures static AWS credentials.
		// False means dynamic mode (AWS default credential chain).
		bedrockStatic bool
		setHeaders    map[string]string
		// wantErr, when set, means CreateInterceptor must fail with it. The
		// remaining expectations are then ignored.
		wantErr            error
		wantCredentialKind intercept.CredentialKind
		wantCredentialHint string
		// Upstream expectations after ProcessRequest. Not checked for Bedrock,
		// which signs via AWS rather than forwarding a key header.
		wantXApiKey       string
		wantAuthorization string
	}{
		{
			name:               "byok_bearer_token",
			pool:               true,
			setHeaders:         map[string]string{"Authorization": "Bearer user-access-token"},
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...en",
			wantAuthorization:  "Bearer user-access-token",
		},
		{
			name:               "byok_api_key",
			pool:               true,
			setHeaders:         map[string]string{"X-Api-Key": "user-api-key"},
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...ey",
			wantXApiKey:        "user-api-key",
		},
		{
			name:       "byok_bearer_and_api_key",
			pool:       true,
			setHeaders: map[string]string{"Authorization": "Bearer user-access-token", "X-Api-Key": "user-api-key"},
			// X-Api-Key takes priority over Authorization.
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...ey",
			wantXApiKey:        "user-api-key",
		},
		{
			name:               "byok_without_pool",
			pool:               false,
			setHeaders:         map[string]string{"X-Api-Key": "user-api-key"},
			wantCredentialKind: intercept.CredentialKindBYOK,
			wantCredentialHint: "us...ey",
			wantXApiKey:        "user-api-key",
		},
		{
			name:               "centralized",
			pool:               true,
			setHeaders:         map[string]string{},
			wantCredentialKind: intercept.CredentialKindCentralized,
			// The pool hasn't handed out a key at CreateInterceptor, so the hint
			// is a placeholder until the failover loop selects one.
			wantCredentialHint: "<failover key>",
			wantXApiKey:        "test-key",
		},
		{
			// Bedrock dynamic mode: no static access key, so the hint is the
			// AWS-credential-chain placeholder.
			name:               "bedrock_dynamic",
			pool:               false,
			bedrock:            true,
			setHeaders:         map[string]string{},
			wantCredentialKind: intercept.CredentialKindCentralized,
			wantCredentialHint: "<aws chain>",
		},
		{
			// Bedrock static mode: the hint masks the access key ID.
			name:               "bedrock_static",
			pool:               false,
			bedrock:            true,
			bedrockStatic:      true,
			setHeaders:         map[string]string{},
			wantCredentialKind: intercept.CredentialKindCentralized,
			wantCredentialHint: "AKIA...MPLE",
		},
		{
			name:       "centralized_without_pool_errors",
			pool:       false,
			setHeaders: map[string]string{},
			wantErr:    ErrNoCredential,
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

			acfg := config.Anthropic{BaseURL: mockUpstream.URL}
			if tc.pool {
				acfg.KeyPool = testutil.SingleKeyPool(config.ProviderAnthropic, "test-key")
			}
			var bedrock *config.AWSBedrock
			if tc.bedrock {
				bedrock = &config.AWSBedrock{Region: "us-west-2", Model: "m", SmallFastModel: "s"}
				if tc.bedrockStatic {
					bedrock.AccessKey = "AKIAIOSFODNN7EXAMPLE"
					bedrock.AccessKeySecret = "wJalrXUtnFEMI-secret-value"
				}
			}
			provider := newTestAnthropic(t, acfg, bedrock)

			body := `{"model": "claude-opus-4-5", "max_tokens": 1024, "messages": [{"role": "user", "content": "hello"}], "stream": false}`
			req := httptest.NewRequest(http.MethodPost, routeMessages, bytes.NewBufferString(body))
			for k, v := range tc.setHeaders {
				req.Header.Set(k, v)
			}
			w := httptest.NewRecorder()

			interceptor, err := provider.CreateInterceptor(w, req, testTracer)
			if tc.wantErr != nil {
				require.ErrorIs(t, err, tc.wantErr)
				require.Nil(t, interceptor)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, interceptor)

			cred := interceptor.Credential()
			assert.Equal(t, tc.wantCredentialKind, cred.Kind(), "credential kind mismatch")
			assert.Equal(t, tc.wantCredentialHint, cred.Hint(), "credential hint mismatch")

			// Bedrock signs via AWS during ProcessRequest (needs real AWS
			// credentials), covered by the integration tests.
			if tc.bedrock {
				return
			}

			interceptor.Setup(slog.Make(), &testutil.MockRecorder{}, nil)
			processReq := httptest.NewRequest(http.MethodPost, routeMessages, nil)
			require.NoError(t, interceptor.ProcessRequest(w, processReq))

			assert.Equal(t, tc.wantXApiKey, receivedHeaders.Get("X-Api-Key"))
			assert.Equal(t, tc.wantAuthorization, receivedHeaders.Get("Authorization"))
		})
	}
}

func TestAnthropic_KeyFailoverConfig(t *testing.T) {
	t.Parallel()

	pool, err := keypool.New(config.ProviderAnthropic, []string{"k0", "k1"}, quartz.NewMock(t), nil)
	require.NoError(t, err)

	p := newTestAnthropic(t, config.Anthropic{KeyPool: pool}, nil)

	cfg := p.KeyFailoverConfig(slog.Make())

	assert.Same(t, pool, cfg.Pool, "Pool must be wired from the provider config")
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
				name:    "non_auth_header",
				headers: map[string]string{"Content-Type": "application/json"},
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
			name              string
			initialHeaders    http.Header
			key               string
			wantAuthorization string
		}{
			{
				name:              "writes_key_to_x_api_key",
				initialHeaders:    http.Header{},
				key:               "centralized-key",
				wantAuthorization: "",
			},
			{
				name:              "overwrites_existing_x_api_key",
				initialHeaders:    http.Header{"X-Api-Key": {"stale"}, "Authorization": {"Bearer stale"}},
				key:               "next-key",
				wantAuthorization: "Bearer stale",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				headers := tc.initialHeaders
				cfg.InjectAuthKey(&headers, tc.key)
				assert.Equal(t, tc.key, headers.Get("X-Api-Key"))
				assert.Equal(t, tc.wantAuthorization, headers.Get("Authorization"))
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
