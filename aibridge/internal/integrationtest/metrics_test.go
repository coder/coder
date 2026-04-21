package integrationtest //nolint:testpackage // tests unexported internals

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	promtest "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/aibridge"
	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/fixtures"
	"github.com/coder/aibridge/internal/testutil"
	"github.com/coder/aibridge/metrics"
)

func TestMetrics_Interception(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		fixture        []byte
		path           string
		headers        http.Header
		expectStatus   string
		expectModel    string
		expectRoute    string
		expectProvider string
		expectClient   aibridge.Client
		allowOverflow  bool // error fixtures may cause retries
	}{
		{
			name:           "ant_simple",
			fixture:        fixtures.AntSimple,
			path:           pathAnthropicMessages,
			expectStatus:   metrics.InterceptionCountStatusCompleted,
			expectModel:    "claude-sonnet-4-0",
			expectRoute:    "/v1/messages",
			expectProvider: config.ProviderAnthropic,
			expectClient:   aibridge.ClientUnknown,
		},
		{
			name:           "ant_error",
			fixture:        fixtures.AntNonStreamError,
			path:           pathAnthropicMessages,
			headers:        http.Header{"User-Agent": []string{"kilo-code/1.2.3"}},
			expectStatus:   metrics.InterceptionCountStatusFailed,
			expectModel:    "claude-sonnet-4-0",
			expectRoute:    "/v1/messages",
			expectProvider: config.ProviderAnthropic,
			expectClient:   aibridge.ClientKilo,
			allowOverflow:  true,
		},
		{
			name:           "ant_simple_claude_code",
			fixture:        fixtures.AntSimple,
			path:           pathAnthropicMessages,
			headers:        http.Header{"User-Agent": []string{"claude-code/1.0.0"}},
			expectStatus:   metrics.InterceptionCountStatusCompleted,
			expectModel:    "claude-sonnet-4-0",
			expectRoute:    "/v1/messages",
			expectProvider: config.ProviderAnthropic,
			expectClient:   aibridge.ClientClaudeCode,
		},
		{
			name:           "oai_chat_simple",
			fixture:        fixtures.OaiChatSimple,
			path:           pathOpenAIChatCompletions,
			headers:        http.Header{"User-Agent": []string{"copilot/1.0.0"}},
			expectStatus:   metrics.InterceptionCountStatusCompleted,
			expectModel:    "gpt-4.1",
			expectRoute:    "/v1/chat/completions",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientCopilotCLI,
		},
		{
			name:           "oai_chat_error",
			fixture:        fixtures.OaiChatNonStreamError,
			path:           pathOpenAIChatCompletions,
			headers:        http.Header{"User-Agent": []string{"githubcopilotchat/0.30.0"}},
			expectStatus:   metrics.InterceptionCountStatusFailed,
			expectModel:    "gpt-4.1",
			expectRoute:    "/v1/chat/completions",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientCopilotVSC,
			allowOverflow:  true,
		},
		{
			name:           "oai_responses_blocking_simple",
			fixture:        fixtures.OaiResponsesBlockingSimple,
			path:           pathOpenAIResponses,
			headers:        http.Header{"X-Cursor-Client-Version": []string{"0.50.0"}},
			expectStatus:   metrics.InterceptionCountStatusCompleted,
			expectModel:    "gpt-4o-mini",
			expectRoute:    "/v1/responses",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientCursor,
		},
		{
			name:           "oai_responses_blocking_error",
			fixture:        fixtures.OaiResponsesBlockingHTTPErr,
			path:           pathOpenAIResponses,
			headers:        http.Header{"User-Agent": []string{"codex/1.0.0"}},
			expectStatus:   metrics.InterceptionCountStatusFailed,
			expectModel:    "gpt-4o-mini",
			expectRoute:    "/v1/responses",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientCodex,
			allowOverflow:  true,
		},
		{
			name:           "oai_responses_streaming_simple",
			fixture:        fixtures.OaiResponsesStreamingSimple,
			path:           pathOpenAIResponses,
			headers:        http.Header{"User-Agent": []string{"zed/0.200.0"}},
			expectStatus:   metrics.InterceptionCountStatusCompleted,
			expectModel:    "gpt-4o-mini",
			expectRoute:    "/v1/responses",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientZed,
		},
		{
			name:           "oai_responses_streaming_error",
			fixture:        fixtures.OaiResponsesStreamingHTTPErr,
			path:           pathOpenAIResponses,
			headers:        http.Header{"Originator": []string{"roo-code"}},
			expectStatus:   metrics.InterceptionCountStatusFailed,
			expectModel:    "gpt-4o-mini",
			expectRoute:    "/v1/responses",
			expectProvider: config.ProviderOpenAI,
			expectClient:   aibridge.ClientRoo,
			allowOverflow:  true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
			upstream.AllowOverflow = tc.allowOverflow

			m := aibridge.NewMetrics(prometheus.NewRegistry())
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withMetrics(m),
			)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, fix.Request(), tc.headers)
			require.NoError(t, err)
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			count := promtest.ToFloat64(m.InterceptionCount.WithLabelValues(
				tc.expectProvider, tc.expectModel, tc.expectStatus, tc.expectRoute, "POST", defaultActorID, string(tc.expectClient)))
			require.Equal(t, 1.0, count)
			require.Equal(t, 1, promtest.CollectAndCount(m.InterceptionDuration))
			require.Equal(t, 1, promtest.CollectAndCount(m.InterceptionCount))
		})
	}
}

func TestMetrics_InterceptionsInflight(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.AntSimple)

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	blockCh := make(chan struct{})

	// Setup a mock HTTP server which blocks until the request is marked as inflight then proceeds.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blockCh
	}))
	t.Cleanup(srv.Close)

	m := aibridge.NewMetrics(prometheus.NewRegistry())
	bridgeServer := newBridgeTestServer(ctx, t, srv.URL,
		withMetrics(m),
	)

	// Make request in background.
	doneCh := make(chan struct{})
	go func() {
		defer close(doneCh)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, bridgeServer.URL+pathAnthropicMessages, bytes.NewReader(fix.Request()))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)
		}
	}()

	// Wait until request is detected as inflight.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(
			m.InterceptionsInflight.WithLabelValues(config.ProviderAnthropic, "claude-sonnet-4-0", "/v1/messages"),
		) == 1
	}, testutil.WaitMedium, testutil.IntervalFast)

	// Unblock request, await completion.
	close(blockCh)
	select {
	case <-doneCh:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}

	// Metric is not updated immediately after request completes, so wait until it is.
	require.Eventually(t, func() bool {
		return promtest.ToFloat64(
			m.InterceptionsInflight.WithLabelValues(config.ProviderAnthropic, "claude-sonnet-4-0", "/v1/messages"),
		) == 0
	}, testutil.WaitMedium, testutil.IntervalFast)
}

func TestMetrics_PassthroughCount(t *testing.T) {
	t.Parallel()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	t.Cleanup(upstream.Close)

	m := aibridge.NewMetrics(prometheus.NewRegistry())
	bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
		withMetrics(m),
	)

	resp, err := bridgeServer.makeRequest(t, http.MethodGet, "/openai/v1/models", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	count := promtest.ToFloat64(m.PassthroughCount.WithLabelValues(
		config.ProviderOpenAI, "/models", "GET"))
	require.Equal(t, 1.0, count)
}

func TestMetrics_PromptCount(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixtures.OaiChatSimple)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

	m := aibridge.NewMetrics(prometheus.NewRegistry())
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withMetrics(m),
	)

	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, fix.Request(), http.Header{"User-Agent": []string{"claude-code/1.0.0"}})
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	prompts := promtest.ToFloat64(m.PromptCount.WithLabelValues(
		config.ProviderOpenAI, "gpt-4.1", defaultActorID, string(aibridge.ClientClaudeCode)))
	require.Equal(t, 1.0, prompts)
}

func TestMetrics_TokenUseCount(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name           string
		fixture        []byte
		reqPath        string
		streaming      bool
		expectProvider string
		expectModel    string
		expectedLabels map[string]float64
	}{
		{
			name:           "openai_responses",
			fixture:        fixtures.OaiResponsesBlockingCachedInputTokens,
			reqPath:        pathOpenAIResponses,
			expectProvider: config.ProviderOpenAI,
			expectModel:    "gpt-4.1",
			expectedLabels: map[string]float64{
				"input":                    129, // 12033 - 11904 cached
				"output":                   44,
				"cache_read_input_tokens":  11904,
				"cache_write_input_tokens": 0,
				"input_cached":             11904,
				"output_reasoning":         0,
				"total_tokens":             12077,
			},
		},
		{
			name:           "anthropic_messages_streaming",
			fixture:        fixtures.AntSingleBuiltinTool,
			reqPath:        pathAnthropicMessages,
			streaming:      true,
			expectProvider: config.ProviderAnthropic,
			expectModel:    "claude-sonnet-4-20250514",
			expectedLabels: map[string]float64{
				"input":                    2,
				"output":                   66,
				"cache_read_input_tokens":  13993,
				"cache_write_input_tokens": 22,
				"cache_read_input":         13993,
				"cache_creation_input":     22,
			},
		},
		{
			name:           "openai_chat_completions",
			fixture:        fixtures.OaiChatSimple,
			reqPath:        pathOpenAIChatCompletions,
			expectProvider: config.ProviderOpenAI,
			expectModel:    "gpt-4.1",
			expectedLabels: map[string]float64{
				"input":                          19,
				"output":                         200,
				"cache_read_input_tokens":        0,
				"cache_write_input_tokens":       0,
				"prompt_cached":                  0,
				"completion_reasoning":           0,
				"completion_accepted_prediction": 0,
				"completion_rejected_prediction": 0,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			m := aibridge.NewMetrics(prometheus.NewRegistry())
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withMetrics(m),
			)

			reqBody := fix.Request()
			if tc.streaming {
				var err error
				reqBody, err = sjson.SetBytes(reqBody, "stream", true)
				require.NoError(t, err)
			}
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.reqPath, reqBody, nil)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, _ = io.ReadAll(resp.Body)

			// metrics are updated asynchronously
			require.Eventually(t, func() bool {
				return promtest.ToFloat64(m.TokenUseCount.WithLabelValues(
					tc.expectProvider, tc.expectModel, "input", defaultActorID, string(aibridge.ClientUnknown))) > 0
			}, testutil.WaitMedium, testutil.IntervalFast)

			for label, expected := range tc.expectedLabels {
				require.Equal(t, expected, promtest.ToFloat64(m.TokenUseCount.WithLabelValues(
					tc.expectProvider, tc.expectModel, label, defaultActorID, string(aibridge.ClientUnknown),
				)), "metric label %q mismatch", label)
			}
		})
	}
}

func TestMetrics_NonInjectedToolUseCount(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	fix := fixtures.Parse(t, fixtures.OaiChatSingleBuiltinTool)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

	m := aibridge.NewMetrics(prometheus.NewRegistry())
	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withMetrics(m),
	)

	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, fix.Request())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	count := promtest.ToFloat64(m.NonInjectedToolUseCount.WithLabelValues(
		config.ProviderOpenAI, "gpt-4.1", "read_file"))
	require.Equal(t, 1.0, count)
}

func TestMetrics_InjectedToolUseCount(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
	t.Cleanup(cancel)

	// First request returns the tool invocation, the second returns the mocked response to the tool result.
	fix := fixtures.Parse(t, fixtures.AntSingleInjectedTool)
	upstream := newMockUpstream(ctx, t, newFixtureResponse(fix), newFixtureToolResponse(fix))

	m := aibridge.NewMetrics(prometheus.NewRegistry())

	// Setup mocked MCP server & tools.
	mockMCP := setupMCPForTest(t, defaultTracer)

	bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
		withMetrics(m),
		withMCP(mockMCP),
	)

	resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, fix.Request())
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	_, err = io.ReadAll(resp.Body)
	require.NoError(t, err)

	// Wait until full roundtrip has completed.
	require.Eventually(t, func() bool {
		return upstream.Calls.Load() == 2
	}, testutil.WaitMedium, testutil.IntervalFast)

	recorder := bridgeServer.Recorder
	require.Len(t, recorder.ToolUsages(), 1)
	require.True(t, recorder.ToolUsages()[0].Injected)
	require.NotNil(t, recorder.ToolUsages()[0].ServerURL)
	actualServerURL := *recorder.ToolUsages()[0].ServerURL

	count := promtest.ToFloat64(m.InjectedToolUseCount.WithLabelValues(
		config.ProviderAnthropic, "claude-sonnet-4-20250514", actualServerURL, mockToolName))
	require.Equal(t, 1.0, count)
}
