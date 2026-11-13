package enterprise_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"golang.org/x/sync/singleflight"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/aibridgetest"
	aibclient "github.com/coder/coder/v2/aibridge/client"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/keypool"
	"github.com/coder/coder/v2/aibridge/recorder"
	aibtracing "github.com/coder/coder/v2/aibridge/tracing"
	agplaibridge "github.com/coder/coder/v2/coderd/aibridge"
	"github.com/coder/coder/v2/coderd/aibridged"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbfake"
	"github.com/coder/coder/v2/coderd/database/dbgen"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/provisionerdserver"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
	"github.com/coder/quartz"
)

// singleKeyPool builds a centralized key pool containing a single key.
func singleKeyPool(t *testing.T, name, key string) *keypool.Pool {
	t.Helper()
	pool, err := keypool.New(name, []string{key}, quartz.NewReal(), nil)
	require.NoError(t, err)
	return pool
}

var testTracer = otel.Tracer("aibridged_inttest")

// TestIntegration is not an exhaustive test against the upstream AI providers' SDKs (see coder/aibridge for those).
// This test validates that:
//   - intercepted requests can be authenticated/authorized
//   - requests can be routed to an appropriate handler
//   - responses can be returned as expected
//   - interceptions are logged, as well as their related prompt, token, and tool calls
//   - MCP server configurations are returned as expected
//   - tracing spans are properly recorded
func TestIntegration(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	tracer := tp.Tracer(t.Name())
	defer func() { _ = tp.Shutdown(t.Context()) }()

	var mcpTokenReceived atomic.Pointer[string]
	mcpHandler := sdkmcp.NewStreamableHTTPHandler(func(*http.Request) *sdkmcp.Server {
		return sdkmcp.NewServer(&sdkmcp.Implementation{
			Name:    "test-mcp-server",
			Version: "1.0.0",
		}, nil)
	}, &sdkmcp.StreamableHTTPOptions{Stateless: true})
	mockMCPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock MCP server received request: %s %s", r.Method, r.URL.Path)
		if auth := r.Header.Get("Authorization"); auth != "" {
			mcpTokenReceived.Store(&auth)
		}
		mcpHandler.ServeHTTP(w, r)
	}))
	t.Cleanup(mockMCPServer.Close)
	t.Logf("Mock MCP server running at: %s", mockMCPServer.URL)

	// Set up mock OpenAI server that returns a tool call response.
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
  "id": "chatcmpl-BwkyFElDIr1egmFyfQ9z4vPBto7m2",
  "object": "chat.completion",
  "created": 1753343279,
  "model": "gpt-4.1-2025-04-14",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": null,
        "tool_calls": [
          {
            "id": "call_KjzAbhiZC6nk81tQzL7pwlpc",
            "type": "function",
            "function": {
              "name": "read_file",
              "arguments": "{\"path\":\"README.md\"}"
            }
          }
        ],
        "refusal": null,
        "annotations": []
      },
      "logprobs": null,
      "finish_reason": "tool_calls"
    }
  ],
  "usage": {
    "prompt_tokens": 60,
    "completion_tokens": 15,
    "total_tokens": 75,
    "prompt_tokens_details": {
      "cached_tokens": 15,
      "audio_tokens": 0
    },
    "completion_tokens_details": {
      "reasoning_tokens": 0,
      "audio_tokens": 0,
      "accepted_prediction_tokens": 0,
      "rejected_prediction_tokens": 0
    }
  },
  "service_tier": "default",
  "system_fingerprint": "fp_b3f1157249"
}`))
	}))
	t.Cleanup(mockOpenAI.Close)

	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
			ExternalAuthConfigs: []*externalauth.Config{
				{
					InstrumentedOAuth2Config: &testutil.OAuth2Config{},
					ID:                       "mock",
					Type:                     "mock",
					DisplayName:              "Mock",
					MCPURL:                   mockMCPServer.URL,
					RefreshGroup:             new(singleflight.Group),
				},
			},
		},
	})

	userClient, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	// Create an API token for the user.
	apiKey, err := userClient.CreateToken(ctx, "me", codersdk.CreateTokenRequest{
		TokenName: fmt.Sprintf("test-key-%d", time.Now().UnixNano()),
		Lifetime:  time.Hour,
		Scope:     codersdk.APIKeyScopeAll,
	})
	require.NoError(t, err)

	// Create external auth link for the user.
	authLink, err := db.InsertExternalAuthLink(ctx, database.InsertExternalAuthLinkParams{
		ProviderID:        "mock",
		UserID:            user.ID,
		CreatedAt:         dbtime.Now(),
		UpdatedAt:         dbtime.Now(),
		OAuthAccessToken:  "test-mock-token",
		OAuthRefreshToken: "test-refresh-token",
		OAuthExpiry:       dbtime.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	// Create aibridge server & client.
	aiBridgeClient, err := api.AGPL.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)
	providers := []aibridge.Provider{aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL, KeyPool: singleKeyPool(t, config.ProviderOpenAI, "test-centralized-key")})}

	// Given: aibridged is started with its server-owned interception pool.
	srv, err := aibridged.New(t.Context(), func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, tracer, nil, nil)
	require.NoError(t, err, "create new aibridged")
	require.NoError(t, srv.ReplaceProviders(ctx, providers))
	t.Cleanup(func() {
		_ = srv.Shutdown(ctx)
	})

	// When: a request is made to aibridged.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{
  "messages": [
    {
      "role": "user",
      "content": "how large is the README.md file in my current path"
    }
  ],
  "model": "gpt-4.1",
  "tools": [
    {
      "type": "function",
      "function": {
        "name": "read_file",
        "description": "Read the contents of a file at the given path.",
        "parameters": {
          "properties": {
            "path": {
              "type": "string"
            }
          },
          "required": [
            "path"
          ],
          "type": "object"
        }
      }
    }
  ]
}`))
	userAgent := "codex_cli_rs/0.87.0"
	require.NoError(t, err, "make request to test server")
	req.Header.Add("Authorization", "Bearer "+apiKey.Key)
	req.Header.Add("Accept", "application/json")
	req.Header.Add("User-Agent", userAgent)

	// When: aibridged handles the request.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Then: the interception & related records are stored.
	interceptions, err := db.GetAIBridgeInterceptions(ctx)
	require.NoError(t, err)
	require.Len(t, interceptions, 1)

	intc0 := interceptions[0]
	keyID, _, err := httpmw.SplitAPIToken(apiKey.Key)
	require.NoError(t, err)
	require.Equal(t, user.ID, intc0.InitiatorID)
	require.True(t, intc0.APIKeyID.Valid)
	require.Equal(t, keyID, intc0.APIKeyID.String)
	require.Equal(t, "openai", intc0.Provider)
	require.Equal(t, "gpt-4.1", intc0.Model)
	require.True(t, intc0.EndedAt.Valid)
	require.False(t, intc0.EndedAt.Time.Before(intc0.StartedAt), "EndedAt should not be before StartedAt")
	require.Less(t, intc0.EndedAt.Time.Sub(intc0.StartedAt), 5*time.Second)
	require.True(t, intc0.Client.Valid)
	require.Equal(t, string(aibclient.Codex), intc0.Client.String)
	require.Equal(t, database.CredentialKindCentralized, intc0.CredentialKind)
	require.Equal(t, "test...-key", intc0.CredentialHint)

	intc0Metadata := gjson.GetBytes(intc0.Metadata.RawMessage, recorder.MetadataUserAgentKey)
	require.Equal(t, userAgent, intc0Metadata.String(), "interception metadata user agent should match request user agent")

	prompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Equal(t, prompts[0].Prompt, "how large is the README.md file in my current path")

	tokens, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.EqualValues(t, tokens[0].InputTokens, 45)
	require.EqualValues(t, tokens[0].OutputTokens, 15)
	require.EqualValues(t, 15, tokens[0].CacheReadInputTokens)

	tools, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.False(t, tools[0].Injected)

	// Then: the MCP server was initialized.
	gotMCPToken := mcpTokenReceived.Load()
	require.NotNil(t, gotMCPToken, "mock MCP server not requested")
	require.Contains(t, *gotMCPToken, authLink.OAuthAccessToken)

	// Then: verify tracing spans were recorded.
	spans := sr.Ended()
	require.NotEmpty(t, spans)
	i := slices.IndexFunc(spans, func(s sdktrace.ReadOnlySpan) bool { return s.Name() == "CachedBridgePool.Acquire" })
	require.NotEqual(t, -1, i, "span named 'CachedBridgePool.Acquire' not found")

	expectAttrs := []attribute.KeyValue{
		attribute.String(aibtracing.InitiatorID, user.ID.String()),
		attribute.String(aibtracing.APIKeyID, keyID),
	}
	require.Equal(t, spans[i].Attributes(), expectAttrs)

	// Check for aibridge spans.
	spanNames := make(map[string]bool)
	for _, span := range spans {
		spanNames[span.Name()] = true
	}

	expectedAibridgeSpans := []string{
		"CachedBridgePool.Acquire",
		"ServerProxyManager.Init",
		"StreamableHTTPServerProxy.Init",
		"StreamableHTTPServerProxy.Init.fetchTools",
		"Intercept",
		"Intercept.CreateInterceptor",
		"Intercept.RecordInterception",
		"Intercept.ProcessRequest",
		"Intercept.ProcessRequest.Upstream",
		"Intercept.RecordPromptUsage",
		"Intercept.RecordTokenUsage",
		"Intercept.RecordToolUsage",
		"Intercept.RecordInterceptionEnded",
	}

	for _, expectedSpan := range expectedAibridgeSpans {
		require.Contains(t, spanNames, expectedSpan)
	}
}

// TestIntegrationWithMetrics validates that Prometheus metrics are correctly incremented
// when requests are processed through aibridged.
func TestIntegrationWithMetrics(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create prometheus registry and metrics.
	registry := prometheus.NewRegistry()
	metrics := aibridge.NewMetrics(registry)

	// Set up mock OpenAI server.
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
  "id": "chatcmpl-test",
  "object": "chat.completion",
  "created": 1753343279,
  "model": "gpt-4.1",
  "choices": [
    {
      "index": 0,
      "message": {
        "role": "assistant",
        "content": "test response"
      },
      "finish_reason": "stop"
    }
  ],
  "usage": {
    "prompt_tokens": 10,
    "completion_tokens": 5,
    "total_tokens": 15
  }
}`))
	}))
	t.Cleanup(mockOpenAI.Close)

	// Database and coderd setup.
	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
		},
	})

	userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	// Create an API token for the user.
	apiKey, err := userClient.CreateToken(ctx, "me", codersdk.CreateTokenRequest{
		TokenName: fmt.Sprintf("test-key-%d", time.Now().UnixNano()),
		Lifetime:  time.Hour,
		Scope:     codersdk.APIKeyScopeCoderAll,
	})
	require.NoError(t, err)

	// Create aibridge client.
	aiBridgeClient, err := api.AGPL.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)
	providers := []aibridge.Provider{aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL, KeyPool: singleKeyPool(t, config.ProviderOpenAI, "test-centralized-key")})}

	// Given: aibridged is started with its server-owned interception pool.
	srv, err := aibridged.New(ctx, func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, testTracer, nil, metrics)
	require.NoError(t, err, "create new aibridged")
	require.NoError(t, srv.ReplaceProviders(ctx, providers))
	t.Cleanup(func() {
		_ = srv.Shutdown(ctx)
	})

	// When: a request is made to aibridged.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(`{
  "messages": [
    {
      "role": "user",
      "content": "test message"
    }
  ],
  "model": "gpt-4.1"
}`))
	require.NoError(t, err, "make request to test server")
	req.Header.Add("Authorization", "Bearer "+apiKey.Key)
	req.Header.Add("Accept", "application/json")

	// When: aibridged handles the request.
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// Then: the interceptions metric should increase to 1.
	// This is not exhaustively checking the available metrics; just an indicative one to prove
	// the plumbing is working.
	require.Eventually(t, func() bool {
		count := promtest.ToFloat64(metrics.InterceptionCount)
		return count == 1
	}, testutil.WaitShort, testutil.IntervalFast, "interceptions_total metric should be 1")
}

// TestIntegrationCircuitBreaker validates that the circuit breaker opens after
// consecutive failures and that the corresponding metrics are exposed.
func TestIntegrationCircuitBreaker(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Create prometheus registry and metrics.
	registry := prometheus.NewRegistry()
	metrics := aibridge.NewMetrics(registry)

	// Set up mock OpenAI server that always returns 503 Service Unavailable.
	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Disable SDK retries.
		w.Header().Set("x-should-retry", "false")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"Service Unavailable.","type":"cf_service_unavailable","code":503}}`))
	}))
	t.Cleanup(mockOpenAI.Close)

	// Set up mock Anthropic server that always returns 529 Overloaded.
	mockAnthropic := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Anthropic uses 529 for overloaded errors.
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`))
	}))
	t.Cleanup(mockAnthropic.Close)

	// Database and coderd setup.
	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{
			Database: db,
			Pubsub:   ps,
		},
	})

	userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)

	// Create an API token for the user.
	apiKey, err := userClient.CreateToken(ctx, "me", codersdk.CreateTokenRequest{
		TokenName: fmt.Sprintf("test-key-%d", time.Now().UnixNano()),
		Lifetime:  time.Hour,
		Scope:     codersdk.APIKeyScopeCoderAll,
	})
	require.NoError(t, err)

	// Create aibridge client.
	aiBridgeClient, err := api.AGPL.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)

	// Create providers with circuit breaker configured to open after 2 failures.
	cbConfig := &config.CircuitBreaker{
		FailureThreshold: 2,
		Interval:         time.Minute,
		Timeout:          time.Minute,
		MaxRequests:      1,
	}
	providers := []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{
			BaseURL:        mockOpenAI.URL,
			KeyPool:        singleKeyPool(t, config.ProviderOpenAI, "test-key"),
			CircuitBreaker: cbConfig,
		}),
		aibridgetest.NewAnthropicProvider(t, aibridge.AnthropicConfig{
			BaseURL:        mockAnthropic.URL,
			KeyPool:        singleKeyPool(t, config.ProviderAnthropic, "test-key"),
			CircuitBreaker: cbConfig,
		}, nil),
	}

	// Given: aibridged is started with its server-owned interception pool.
	srv, err := aibridged.New(ctx, func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, testTracer, nil, metrics)
	require.NoError(t, err, "create new aibridged")
	require.NoError(t, srv.ReplaceProviders(ctx, providers))
	t.Cleanup(func() {
		_ = srv.Shutdown(ctx)
	})

	// Test OpenAI circuit breaker.
	openaiRequestBody := `{"messages":[{"role":"user","content":"test"}],"model":"gpt-4"}`
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/openai/v1/chat/completions", bytes.NewBufferString(openaiRequestBody))
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+apiKey.Key)
		req.Header.Add("Accept", "application/json")

		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		t.Logf("OpenAI request %d: status=%d", i+1, rec.Code)
	}

	// Test Anthropic circuit breaker.
	anthropicRequestBody := `{"messages":[{"role":"user","content":"test"}],"model":"claude-3-5-sonnet-20241022","max_tokens":100}`
	for i := 0; i < 3; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/anthropic/v1/messages", bytes.NewBufferString(anthropicRequestBody))
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+apiKey.Key)
		req.Header.Add("Accept", "application/json")

		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		t.Logf("Anthropic request %d: status=%d", i+1, rec.Code)
	}

	// Then: the circuit breaker metrics should reflect that both circuits opened.

	// OpenAI circuit breaker should have tripped (state=1 means open).
	openaiTrips := promtest.ToFloat64(metrics.CircuitBreakerTrips.WithLabelValues("openai", "/v1/chat/completions", "gpt-4"))
	require.Equal(t, 1.0, openaiTrips, "OpenAI CircuitBreakerTrips should be 1")

	openaiState := promtest.ToFloat64(metrics.CircuitBreakerState.WithLabelValues("openai", "/v1/chat/completions", "gpt-4"))
	require.Equal(t, 1.0, openaiState, "OpenAI CircuitBreakerState should be 1 (open)")

	// Anthropic circuit breaker should have tripped.
	anthropicTrips := promtest.ToFloat64(metrics.CircuitBreakerTrips.WithLabelValues("anthropic", "/v1/messages", "claude-3-5-sonnet-20241022"))
	require.Equal(t, 1.0, anthropicTrips, "Anthropic CircuitBreakerTrips should be 1")

	anthropicState := promtest.ToFloat64(metrics.CircuitBreakerState.WithLabelValues("anthropic", "/v1/messages", "claude-3-5-sonnet-20241022"))
	require.Equal(t, 1.0, anthropicState, "Anthropic CircuitBreakerState should be 1 (open)")
}

// TestIntegrationRecordsUpstreamError validates that a failed interception
// persists a categorized upstream error on its database row.
func TestIntegrationRecordsUpstreamError(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	// Mock upstreams that reject with 401 Unauthorized.
	unauthorizedHandler := func(body string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("x-should-retry", "false")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(body))
		}
	}
	mockOpenAI := httptest.NewServer(unauthorizedHandler(`{"error":{"message":"Incorrect API key","type":"invalid_request_error","code":"invalid_api_key"}}`))
	t.Cleanup(mockOpenAI.Close)
	mockAnthropic := httptest.NewServer(unauthorizedHandler(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	t.Cleanup(mockAnthropic.Close)

	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{Database: db, Pubsub: ps},
	})

	userClient, _ := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
	apiKey, err := userClient.CreateToken(ctx, "me", codersdk.CreateTokenRequest{
		TokenName: fmt.Sprintf("test-key-%d", time.Now().UnixNano()),
		Lifetime:  time.Hour,
		Scope:     codersdk.APIKeyScopeCoderAll,
	})
	require.NoError(t, err)

	aiBridgeClient, err := api.AGPL.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)
	providers := []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL, KeyPool: singleKeyPool(t, config.ProviderOpenAI, "test-key")}),
		aibridgetest.NewAnthropicProvider(t, aibridge.AnthropicConfig{BaseURL: mockAnthropic.URL, KeyPool: singleKeyPool(t, config.ProviderAnthropic, "test-key")}, nil),
	}
	srv, err := aibridged.New(ctx, func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, testTracer, nil, nil)
	require.NoError(t, err, "create new aibridged")
	require.NoError(t, srv.ReplaceProviders(ctx, providers))
	t.Cleanup(func() { _ = srv.Shutdown(ctx) })

	cases := []struct {
		name string
		path string
		body string
	}{
		{"openai_blocking", "/openai/v1/chat/completions", `{"messages":[{"role":"user","content":"test"}],"model":"gpt-4","stream":false}`},
		{"openai_streaming", "/openai/v1/chat/completions", `{"messages":[{"role":"user","content":"test"}],"model":"gpt-4","stream":true}`},
		{"anthropic_blocking", "/anthropic/v1/messages", `{"messages":[{"role":"user","content":"test"}],"model":"claude-3-5-sonnet-20241022","max_tokens":100,"stream":false}`},
		{"anthropic_streaming", "/anthropic/v1/messages", `{"messages":[{"role":"user","content":"test"}],"model":"claude-3-5-sonnet-20241022","max_tokens":100,"stream":true}`},
	}

	// Requests are fired sequentially: they share the centralized key pool and
	// database, and the aggregate assertion below depends on all of them.
	for _, tc := range cases {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, tc.path, bytes.NewBufferString(tc.body))
		require.NoError(t, err)
		req.Header.Add("Authorization", "Bearer "+apiKey.Key)
		req.Header.Add("Accept", "application/json")

		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)
		t.Logf("%s request: status=%d", tc.name, rec.Code)
	}

	// The interception rows should record the unauthorized upstream failure.
	// Both blocking and streaming interceptors surface the centralized key-pool
	// permanent exhaustion, which categorizes as unauthorized.
	intcs, err := db.GetAIBridgeInterceptions(ctx)
	require.NoError(t, err)
	require.Len(t, intcs, len(cases))
	for _, intc := range intcs {
		require.True(t, intc.EndedAt.Valid, "interception %s should be ended", intc.ID)
		require.True(t, intc.ErrorType.Valid, "interception %s should record an error type", intc.ID)
		require.Equal(t, database.AibridgeInterceptionErrorTypeUnauthorized, intc.ErrorType.AIBridgeInterceptionErrorType)
		require.True(t, intc.ErrorMessage.Valid, "interception %s should record an error message", intc.ID)
		require.NotEmpty(t, intc.ErrorMessage.String)
	}
}

// TestIntegrationResolvedOrganization validates that the organization coderd
// resolves in IsAuthorized is used for the whole request: the budget check,
// the interception record, and the effective group, price and spend that cost
// records. Each case uses its own user, who is a member of their own orgs A
// and B, against a shared deployment.
func TestIntegrationResolvedOrganization(t *testing.T) {
	t.Parallel()

	ctx := testutil.Context(t, testutil.WaitLong)

	mockOpenAI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
  "id": "chatcmpl-resolved-org",
  "object": "chat.completion",
  "created": 1753343279,
  "model": "gpt-4.1-2025-04-14",
  "choices": [{"index": 0, "message": {"role": "assistant", "content": "hello"}, "finish_reason": "stop"}],
  "usage": {"prompt_tokens": 60, "completion_tokens": 15, "total_tokens": 75}
}`))
	}))
	t.Cleanup(mockOpenAI.Close)

	db, ps := dbtestutil.NewDB(t)
	client, _, api, firstUser := coderdenttest.NewWithAPI(t, &coderdenttest.Options{
		Options: &coderdtest.Options{Database: db, Pubsub: ps},
	})

	// Given: the model has a site-wide custom price. Each case also sets a
	// different price in its orgs A and B, so the price used identifies the
	// organization.
	const siteWideInputPrice, orgAInputPrice, orgBInputPrice int64 = 2_000_000, 3_000_000, 4_000_000
	aiProvider := dbgen.AIProvider(t, db, database.AIProvider{
		Name: config.ProviderOpenAI,
		Type: database.AIProviderTypeOpenai,
	})
	priceSeed, err := json.Marshal([]map[string]any{{
		"provider":     config.ProviderOpenAI,
		"model":        "gpt-4.1",
		"input_price":  siteWideInputPrice,
		"output_price": 8_000_000,
	}})
	require.NoError(t, err)
	require.NoError(t, db.UpsertAIModelPrices(ctx, database.UpsertAIModelPricesParams{Seed: priceSeed, Source: database.AIModelPriceSourceCustom}))

	aiBridgeClient, err := api.AGPL.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)
	srv, err := aibridged.New(ctx, func(context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, testutil.Logger(t), testTracer, nil, nil)
	require.NoError(t, err, "create new aibridged")
	require.NoError(t, srv.ReplaceProviders(ctx, []aibridge.Provider{
		aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL, KeyPool: singleKeyPool(t, config.ProviderOpenAI, "test-key")}),
	}))
	t.Cleanup(func() { _ = srv.Shutdown(ctx) })

	// fixture is the state each case starts from.
	type fixture struct {
		userID         uuid.UUID
		groupA, groupB database.Group
		workspace      database.WorkspaceTable
	}

	// newRequest builds the chat completion request every case sends, without
	// credentials. Direct requests add a token with directRequest, and delegated
	// requests authenticate through the context instead.
	newRequest := func(ctx context.Context, t *testing.T) *http.Request {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "/openai/v1/chat/completions",
			bytes.NewBufferString(`{"messages":[{"role":"user","content":"hi"}],"model":"gpt-4.1"}`))
		require.NoError(t, err)
		req.Header.Add("Accept", "application/json")
		return req
	}
	// directRequest authenticates with a token in the Authorization header.
	directRequest := func(ctx context.Context, t *testing.T, token string) *http.Request {
		req := newRequest(ctx, t)
		req.Header.Add("Authorization", "Bearer "+token)
		return req
	}

	tests := []struct {
		name string
		// initialSpendA is the spend already recorded in group A before the
		// request.
		initialSpendA int64
		// request builds the request the user sends.
		request func(ctx context.Context, t *testing.T, f fixture) *http.Request
		// wantGroup returns the group, and so the organization, the request is
		// attributed to.
		wantGroup      func(f fixture) database.Group
		wantSource     database.AIBridgeInterceptionOrganizationSource
		wantInputPrice int64
		wantWorkspace  bool
	}{
		{
			// When: the user sends a normal API token, the request resolves
			// to org A through the site-wide effective group, because group A
			// has the higher limit.
			name: "effective group",
			request: func(ctx context.Context, t *testing.T, f fixture) *http.Request {
				_, token := dbgen.APIKey(t, db, database.APIKey{UserID: f.userID})
				return directRequest(ctx, t, token)
			},
			wantGroup:      func(f fixture) database.Group { return f.groupA },
			wantSource:     database.AibridgeInterceptionOrganizationSourceEffectiveGroup,
			wantInputPrice: orgAInputPrice,
		},
		{
			// When: the user sends the session token of a workspace in org B,
			// the request resolves to org B instead of the effective group.
			name: "workspace session token",
			request: func(ctx context.Context, t *testing.T, f fixture) *http.Request {
				_, token := dbgen.APIKey(t, db, database.APIKey{
					UserID:    f.userID,
					TokenName: provisionerdserver.WorkspaceSessionTokenName(f.userID, f.workspace.ID),
				})
				return directRequest(ctx, t, token)
			},
			wantGroup:      func(f fixture) database.Group { return f.groupB },
			wantSource:     database.AibridgeInterceptionOrganizationSourceWorkspace,
			wantInputPrice: orgBInputPrice,
			wantWorkspace:  true,
		},
		{
			// When: a delegated chatd request carries org B as the chat's
			// organization, the request resolves to org B instead of the
			// effective group.
			name: "delegated organization",
			request: func(ctx context.Context, t *testing.T, f fixture) *http.Request {
				key, _ := dbgen.APIKey(t, db, database.APIKey{UserID: f.userID})
				ctx = agplaibridge.WithDelegatedAPIKeyID(ctx, key.ID)
				ctx = agplaibridge.WithDelegatedAttribution(ctx, agplaibridge.Attribution{DelegatedOrganizationID: f.groupB.OrganizationID})
				return newRequest(ctx, t)
			},
			wantGroup:      func(f fixture) database.Group { return f.groupB },
			wantSource:     database.AibridgeInterceptionOrganizationSourceChatd,
			wantInputPrice: orgBInputPrice,
		},
		{
			// When: a delegated chatd request carries org B as the chat's
			// organization and the chat's workspace in org B, the request
			// resolves to org B through the delegated organization, and the
			// workspace is recorded on the interception.
			name: "delegated organization with workspace",
			request: func(ctx context.Context, t *testing.T, f fixture) *http.Request {
				key, _ := dbgen.APIKey(t, db, database.APIKey{UserID: f.userID})
				ctx = agplaibridge.WithDelegatedAPIKeyID(ctx, key.ID)
				ctx = agplaibridge.WithDelegatedAttribution(ctx, agplaibridge.Attribution{
					WorkspaceID:             f.workspace.ID,
					DelegatedOrganizationID: f.groupB.OrganizationID,
				})
				return newRequest(ctx, t)
			},
			wantGroup:      func(f fixture) database.Group { return f.groupB },
			wantSource:     database.AibridgeInterceptionOrganizationSourceChatd,
			wantInputPrice: orgBInputPrice,
			wantWorkspace:  true,
		},
		{
			// When: the user is over group A's 100 USD limit and sends the session
			// token of a workspace in org B. Unlike the other cases, this checks that
			// IsBudgetExceeded uses the resolved org B, not the site-wide group A, so
			// the request is allowed.
			name:          "budget follows the resolved org",
			initialSpendA: 150_000_000,
			request: func(ctx context.Context, t *testing.T, f fixture) *http.Request {
				_, token := dbgen.APIKey(t, db, database.APIKey{
					UserID:    f.userID,
					TokenName: provisionerdserver.WorkspaceSessionTokenName(f.userID, f.workspace.ID),
				})
				return directRequest(ctx, t, token)
			},
			wantGroup:      func(f fixture) database.Group { return f.groupB },
			wantSource:     database.AibridgeInterceptionOrganizationSourceWorkspace,
			wantInputPrice: orgBInputPrice,
			wantWorkspace:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctx := testutil.Context(t, testutil.WaitLong)
			now := dbtime.Now()

			// Given: the user is in orgs A and B, with a budgeted group in
			// each. Group A has the higher limit, so it wins site-wide. Each
			// org has its own price for the model.
			_, user := coderdtest.CreateAnotherUser(t, client, firstUser.OrganizationID)
			f := fixture{userID: user.ID}
			budgetedGroup := func(limitMicros, inputPrice int64) database.Group {
				org := dbgen.Organization(t, db, database.Organization{})
				dbgen.OrganizationMember(t, db, database.OrganizationMember{OrganizationID: org.ID, UserID: f.userID})
				group := dbgen.Group(t, db, database.Group{OrganizationID: org.ID})
				dbgen.GroupMember(t, db, database.GroupMemberTable{UserID: f.userID, GroupID: group.ID})
				_, err := db.UpsertGroupAIBudget(ctx, database.UpsertGroupAIBudgetParams{
					GroupID:          group.ID,
					SpendLimitMicros: limitMicros,
				})
				require.NoError(t, err)
				require.NoError(t, db.UpsertOrganizationAIProviderModelConfig(ctx, database.UpsertOrganizationAIProviderModelConfigParams{
					OrganizationID: org.ID,
					ProviderID:     aiProvider.ID,
					Model:          "gpt-4.1",
					InputPrice:     sql.NullInt64{Int64: inputPrice, Valid: true},
					OutputPrice:    sql.NullInt64{Int64: 8_000_000, Valid: true},
				}))
				return group
			}
			f.groupA = budgetedGroup(100_000_000, orgAInputPrice)
			f.groupB = budgetedGroup(10_000_000, orgBInputPrice)

			// Given: a workspace in org B.
			f.workspace = dbfake.WorkspaceBuild(t, db, database.WorkspaceTable{
				OrganizationID: f.groupB.OrganizationID,
				OwnerID:        f.userID,
			}).Do().Workspace

			// Given: the spend already recorded in group A, if any.
			if tt.initialSpendA > 0 {
				_, err := db.IncrementUserAIDailySpend(ctx, database.IncrementUserAIDailySpendParams{
					UserID:           f.userID,
					EffectiveGroupID: f.groupA.ID,
					Day:              now,
					CostMicros:       tt.initialSpendA,
				})
				require.NoError(t, err)
			}

			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, tt.request(ctx, t, f))
			require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

			// Then: the request is recorded with the resolved organization
			// and its source.
			wantGroup := tt.wantGroup(f)
			all, err := db.GetAIBridgeInterceptions(ctx)
			require.NoError(t, err)
			var interceptions []database.AIBridgeInterception
			for _, intc := range all {
				if intc.InitiatorID == f.userID {
					interceptions = append(interceptions, intc)
				}
			}
			require.Len(t, interceptions, 1)
			intc := interceptions[0]
			require.Equal(t, uuid.NullUUID{UUID: wantGroup.OrganizationID, Valid: true}, intc.OrganizationID)
			require.Equal(t, database.NullAIBridgeInterceptionOrganizationSource{
				AIBridgeInterceptionOrganizationSource: tt.wantSource,
				Valid:                                  true,
			}, intc.OrganizationSource)
			if tt.wantWorkspace {
				require.Equal(t, uuid.NullUUID{UUID: f.workspace.ID, Valid: true}, intc.WorkspaceID)
			}

			// Then: cost prices the request with the resolved org's price and
			// attributes the token usage and the spend to that org's group.
			// The other group's spend is unchanged.
			tokens, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, intc.ID)
			require.NoError(t, err)
			require.Len(t, tokens, 1)
			require.Equal(t, uuid.NullUUID{UUID: wantGroup.ID, Valid: true}, tokens[0].EffectiveGroupID)
			require.Equal(t, sql.NullInt64{Int64: tt.wantInputPrice, Valid: true}, tokens[0].InputPriceMicros)
			require.True(t, tokens[0].CostMicros.Valid)
			require.Positive(t, tokens[0].CostMicros.Int64)

			spendSince := func(groupID uuid.UUID) int64 {
				spend, err := db.GetUserAISpendSince(ctx, database.GetUserAISpendSinceParams{
					UserID:           f.userID,
					EffectiveGroupID: groupID,
					PeriodStart:      dbtime.StartOfDay(now),
				})
				require.NoError(t, err)
				return spend.SpendMicros
			}
			cost := tokens[0].CostMicros.Int64
			if wantGroup.ID == f.groupA.ID {
				require.Equal(t, tt.initialSpendA+cost, spendSince(f.groupA.ID))
				require.Zero(t, spendSince(f.groupB.ID))
			} else {
				require.Equal(t, cost, spendSince(f.groupB.ID))
				require.Equal(t, tt.initialSpendA, spendSince(f.groupA.ID))
			}
		})
	}
}
