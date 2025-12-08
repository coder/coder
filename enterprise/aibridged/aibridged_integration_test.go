package aibridged_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"

	"github.com/coder/aibridge"
	aibtracing "github.com/coder/aibridge/tracing"
	"github.com/coder/coder/v2/coderd/coderdtest"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/coderd/database/dbtestutil"
	"github.com/coder/coder/v2/coderd/database/dbtime"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/aibridged"
	"github.com/coder/coder/v2/enterprise/coderd/coderdenttest"
	"github.com/coder/coder/v2/testutil"
)

var testTracer = otel.Tracer("aibridged_test")

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

	// Create mock MCP server.
	var mcpTokenReceived string
	mockMCPServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Logf("Mock MCP server received request: %s %s", r.Method, r.URL.Path)

		if r.Method == http.MethodPost && r.URL.Path == "/" {
			// Mark that init was called.
			mcpTokenReceived = r.Header.Get("Authorization")
			t.Log("MCP init request received")

			// Return a basic MCP init response.
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", "test-session-123")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"jsonrpc": "2.0",
				"id": 1,
				"result": {
					"protocolVersion": "2024-11-05",
					"capabilities": {},
					"serverInfo": {
						"name": "test-mcp-server",
						"version": "1.0.0"
					}
				}
			}`))
		}
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
	authLink, err := db.InsertExternalAuthLink(dbauthz.AsSystemRestricted(ctx), database.InsertExternalAuthLinkParams{
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
	aiBridgeClient, err := api.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)
	providers := []aibridge.Provider{aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL})}
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger, nil, tracer)
	require.NoError(t, err)

	// Given: aibridged is started.
	srv, err := aibridged.New(t.Context(), pool, func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, tracer, nil)
	require.NoError(t, err, "create new aibridged")
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
	require.NoError(t, err, "make request to test server")
	req.Header.Add("Authorization", "Bearer "+apiKey.Key)
	req.Header.Add("Accept", "application/json")

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
	require.True(t, intc0.StartedAt.Before(intc0.EndedAt.Time))
	require.Less(t, intc0.EndedAt.Time.Sub(intc0.StartedAt), 5*time.Second)

	prompts, err := db.GetAIBridgeUserPromptsByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, prompts, 1)
	require.Equal(t, prompts[0].Prompt, "how large is the README.md file in my current path")

	tokens, err := db.GetAIBridgeTokenUsagesByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, tokens, 1)
	require.EqualValues(t, tokens[0].InputTokens, 45)
	require.EqualValues(t, tokens[0].OutputTokens, 15)
	require.EqualValues(t, gjson.Get(string(tokens[0].Metadata.RawMessage), "prompt_cached").Int(), 15)

	tools, err := db.GetAIBridgeToolUsagesByInterceptionID(ctx, interceptions[0].ID)
	require.NoError(t, err)
	require.Len(t, tools, 1)
	require.False(t, tools[0].Injected)

	// Then: the MCP server was initialized.
	require.Contains(t, mcpTokenReceived, authLink.OAuthAccessToken, "mock MCP server not requested")

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
	aiBridgeClient, err := api.CreateInMemoryAIBridgeServer(ctx)
	require.NoError(t, err)

	logger := testutil.Logger(t)
	providers := []aibridge.Provider{aibridge.NewOpenAIProvider(aibridge.OpenAIConfig{BaseURL: mockOpenAI.URL})}

	// Create pool with metrics.
	pool, err := aibridged.NewCachedBridgePool(aibridged.DefaultPoolOptions, providers, logger, metrics, testTracer)
	require.NoError(t, err)

	// Given: aibridged is started.
	srv, err := aibridged.New(ctx, pool, func(ctx context.Context) (aibridged.DRPCClient, error) {
		return aiBridgeClient, nil
	}, logger, testTracer, nil)
	require.NoError(t, err, "create new aibridged")
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
