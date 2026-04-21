package integrationtest //nolint:testpackage // tests unexported internals

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/fixtures"
	"github.com/coder/aibridge/internal/testutil"
	"github.com/coder/aibridge/tracing"
)

// expect 'count' amount of traces named 'name' with status 'status'
type expectTrace struct {
	name   string
	count  int
	status codes.Code
}

func setupTracer(t *testing.T) (*tracetest.SpanRecorder, oteltrace.Tracer) {
	t.Helper()

	sr := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(sr))
	t.Cleanup(func() {
		_ = tp.Shutdown(t.Context())
	})

	return sr, tp.Tracer(t.Name())
}

func TestTraceAnthropic(t *testing.T) {
	t.Parallel()

	expectNonStreaming := []expectTrace{
		{"Intercept", 1, codes.Unset},
		{"Intercept.CreateInterceptor", 1, codes.Unset},
		{"Intercept.RecordInterception", 1, codes.Unset},
		{"Intercept.ProcessRequest", 1, codes.Unset},
		{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
		{"Intercept.RecordPromptUsage", 1, codes.Unset},
		{"Intercept.RecordTokenUsage", 1, codes.Unset},
		{"Intercept.RecordToolUsage", 1, codes.Unset},
		{"Intercept.RecordModelThought", 1, codes.Unset},
		{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
	}

	expectStreaming := []expectTrace{
		{"Intercept", 1, codes.Unset},
		{"Intercept.CreateInterceptor", 1, codes.Unset},
		{"Intercept.RecordInterception", 1, codes.Unset},
		{"Intercept.ProcessRequest", 1, codes.Unset},
		{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
		{"Intercept.RecordPromptUsage", 1, codes.Unset},
		{"Intercept.RecordTokenUsage", 2, codes.Unset},
		{"Intercept.RecordToolUsage", 1, codes.Unset},
		{"Intercept.RecordModelThought", 1, codes.Unset},
		{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
	}

	cases := []struct {
		name      string
		fixture   []byte
		streaming bool
		bedrock   bool
		expect    []expectTrace
	}{
		{
			name:    "trace_anthr_non_streaming",
			expect:  expectNonStreaming,
			fixture: fixtures.AntSingleBuiltinTool,
		},
		{
			name:    "trace_bedrock_non_streaming",
			bedrock: true,
			expect:  expectNonStreaming,
			fixture: fixtures.AntSingleBuiltinTool,
		},
		{
			name:      "trace_anthr_streaming",
			streaming: true,
			expect:    expectStreaming,
			fixture:   fixtures.AntSingleBuiltinTool,
		},
		{
			name:      "trace_bedrock_streaming",
			streaming: true,
			bedrock:   true,
			expect:    expectStreaming,
			fixture:   fixtures.AntSingleBuiltinTool,
		},
		{
			name:    "trace_multi_thinking_non_streaming",
			fixture: fixtures.AntMultiThinkingBuiltinTool,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.RecordToolUsage", 1, codes.Unset},
				{"Intercept.RecordModelThought", 2, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_multi_thinking_streaming",
			fixture:   fixtures.AntMultiThinkingBuiltinTool,
			streaming: true,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 2, codes.Unset},
				{"Intercept.RecordToolUsage", 1, codes.Unset},
				{"Intercept.RecordModelThought", 2, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			sr, tracer := setupTracer(t)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			opts := []bridgeOption{
				withTracer(tracer),
			}
			if tc.bedrock {
				opts = append(opts, withProvider(providerBedrock))
			}
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, opts...)

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			bridgeServer.Close()

			require.Equal(t, 1, len(bridgeServer.Recorder.RecordedInterceptions()))
			intcID := bridgeServer.Recorder.RecordedInterceptions()[0].ID

			model := gjson.Get(string(reqBody), "model").Str
			if tc.bedrock {
				model = "beddel"
			}

			totalCount := 0
			for _, e := range tc.expect {
				totalCount += e.count
			}

			attrs := []attribute.KeyValue{
				attribute.String(tracing.RequestPath, "/anthropic/v1/messages"),
				attribute.String(tracing.InterceptionID, intcID),
				attribute.String(tracing.Provider, config.ProviderAnthropic),
				attribute.String(tracing.Model, model),
				attribute.String(tracing.InitiatorID, defaultActorID),
				attribute.Bool(tracing.Streaming, tc.streaming),
				attribute.Bool(tracing.IsBedrock, tc.bedrock),
			}

			require.Len(t, sr.Ended(), totalCount)
			verifyTraces(t, sr, tc.expect, attrs)
		})
	}
}

func TestTraceAnthropicErr(t *testing.T) {
	t.Parallel()

	expectNonStream := []expectTrace{
		{"Intercept", 1, codes.Error},
		{"Intercept.CreateInterceptor", 1, codes.Unset},
		{"Intercept.RecordInterception", 1, codes.Unset},
		{"Intercept.ProcessRequest", 1, codes.Error},
		{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
		{"Intercept.ProcessRequest.Upstream", 1, codes.Error},
	}

	expectStreaming := []expectTrace{
		{"Intercept", 1, codes.Error},
		{"Intercept.CreateInterceptor", 1, codes.Unset},
		{"Intercept.RecordInterception", 1, codes.Unset},
		{"Intercept.ProcessRequest", 1, codes.Error},
		{"Intercept.RecordPromptUsage", 1, codes.Unset},
		{"Intercept.RecordTokenUsage", 1, codes.Unset},
		{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
		{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
	}

	cases := []struct {
		name       string
		fixture    []byte
		streaming  bool
		bedrock    bool
		expectCode int // expected status code for non-streaming responses
		expect     []expectTrace
	}{
		{
			name:       "anthr_non_streaming_err",
			fixture:    fixtures.AntNonStreamError,
			expectCode: http.StatusBadRequest,
			expect:     expectNonStream,
		},
		{
			name:      "anthr_streaming_err",
			fixture:   fixtures.AntMidStreamError,
			streaming: true,
			expect:    expectStreaming,
		},
		{
			name:       "bedrock_non_streaming_err",
			fixture:    fixtures.AntNonStreamError,
			bedrock:    true,
			expectCode: http.StatusBadRequest,
			expect:     expectNonStream,
		},
		{
			name:      "bedrock_streaming_err",
			fixture:   fixtures.AntMidStreamError,
			streaming: true,
			bedrock:   true,
			expect:    expectStreaming,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			sr, tracer := setupTracer(t)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			opts := []bridgeOption{
				withTracer(tracer),
			}
			if tc.bedrock {
				opts = append(opts, withProvider(providerBedrock))
			}
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, opts...)

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			if tc.streaming {
				require.Equal(t, http.StatusOK, resp.StatusCode)
			} else {
				require.Equal(t, tc.expectCode, resp.StatusCode)
			}
			bridgeServer.Close()

			require.Equal(t, 1, len(bridgeServer.Recorder.RecordedInterceptions()))
			intcID := bridgeServer.Recorder.RecordedInterceptions()[0].ID

			totalCount := 0
			for _, e := range tc.expect {
				totalCount += e.count
			}
			for _, s := range sr.Ended() {
				t.Logf("SPAN: %v", s.Name())
			}
			require.Len(t, sr.Ended(), totalCount)

			model := gjson.Get(string(reqBody), "model").Str
			if tc.bedrock {
				model = "beddel"
			}

			attrs := []attribute.KeyValue{
				attribute.String(tracing.RequestPath, "/anthropic/v1/messages"),
				attribute.String(tracing.InterceptionID, intcID),
				attribute.String(tracing.Provider, config.ProviderAnthropic),
				attribute.String(tracing.Model, model),
				attribute.String(tracing.InitiatorID, defaultActorID),
				attribute.Bool(tracing.Streaming, tc.streaming),
				attribute.Bool(tracing.IsBedrock, tc.bedrock),
			}

			verifyTraces(t, sr, tc.expect, attrs)
		})
	}
}

func TestInjectedToolsTrace(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		streaming      bool
		bedrock        bool
		fixture        []byte
		path           string
		expectModel    string
		expectProvider string
		opts           []bridgeOption
	}{
		{
			name:           "anthr_blocking",
			streaming:      false,
			fixture:        fixtures.AntSingleInjectedTool,
			path:           pathAnthropicMessages,
			expectModel:    "claude-sonnet-4-20250514",
			expectProvider: config.ProviderAnthropic,
		},
		{
			name:           "anthr_streaming",
			streaming:      true,
			fixture:        fixtures.AntSingleInjectedTool,
			path:           pathAnthropicMessages,
			expectModel:    "claude-sonnet-4-20250514",
			expectProvider: config.ProviderAnthropic,
		},
		{
			name:           "bedrock_blocking",
			streaming:      false,
			bedrock:        true,
			fixture:        fixtures.AntSingleInjectedTool,
			path:           pathAnthropicMessages,
			expectModel:    "beddel",
			expectProvider: config.ProviderAnthropic,
			opts:           []bridgeOption{withProvider(providerBedrock)},
		},
		{
			name:           "bedrock_streaming",
			streaming:      true,
			bedrock:        true,
			fixture:        fixtures.AntSingleInjectedTool,
			path:           pathAnthropicMessages,
			expectModel:    "beddel",
			expectProvider: config.ProviderAnthropic,
			opts:           []bridgeOption{withProvider(providerBedrock)},
		},
		{
			name:           "openai_blocking",
			streaming:      false,
			fixture:        fixtures.OaiChatSingleInjectedTool,
			path:           pathOpenAIChatCompletions,
			expectModel:    "gpt-4.1",
			expectProvider: config.ProviderOpenAI,
		},
		{
			name:           "openai_streaming",
			streaming:      true,
			fixture:        fixtures.OaiChatSingleInjectedTool,
			path:           pathOpenAIChatCompletions,
			expectModel:    "gpt-4.1",
			expectProvider: config.ProviderOpenAI,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			sr, tracer := setupTracer(t)

			var validatorFn func(*http.Request, []byte)
			if tc.expectProvider == config.ProviderAnthropic {
				validatorFn = anthropicToolResultValidator(t)
			} else {
				validatorFn = openaiChatToolResultValidator(t)
			}

			bridgeServer, mockMCP, resp := setupInjectedToolTest(
				t, tc.fixture, tc.streaming, tracer,
				tc.path, validatorFn, tc.opts...,
			)
			defer resp.Body.Close()

			require.Len(t, bridgeServer.Recorder.RecordedInterceptions(), 1)
			intcID := bridgeServer.Recorder.RecordedInterceptions()[0].ID

			tool := mockMCP.ListTools()[0]

			attrs := []attribute.KeyValue{
				attribute.String(tracing.RequestPath, tc.path),
				attribute.String(tracing.InterceptionID, intcID),
				attribute.String(tracing.Provider, tc.expectProvider),
				attribute.String(tracing.Model, tc.expectModel),
				attribute.String(tracing.InitiatorID, defaultActorID),
				attribute.String(tracing.MCPInput, `{"owner":"admin"}`),
				attribute.String(tracing.MCPToolName, "coder_list_workspaces"),
				attribute.String(tracing.MCPServerName, tool.ServerName),
				attribute.String(tracing.MCPServerURL, tool.ServerURL),
				attribute.Bool(tracing.Streaming, tc.streaming),
			}
			if tc.expectProvider == config.ProviderAnthropic {
				attrs = append(attrs, attribute.Bool(tracing.IsBedrock, tc.bedrock))
			}

			verifyTraces(t, sr, []expectTrace{{"Intercept.ProcessRequest.ToolCall", 1, codes.Unset}}, attrs)
		})
	}
}

func TestTraceOpenAI(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		fixture   []byte
		streaming bool
		path      string

		expect []expectTrace
	}{
		{
			name:      "trace_openai_chat_streaming",
			fixture:   fixtures.OaiChatSimple,
			streaming: true,
			path:      pathOpenAIChatCompletions,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_chat_blocking",
			fixture:   fixtures.OaiChatSimple,
			streaming: false,
			path:      pathOpenAIChatCompletions,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_streaming",
			fixture:   fixtures.OaiResponsesStreamingSimple,
			streaming: true,
			path:      pathOpenAIResponses,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_blocking",
			fixture:   fixtures.OaiResponsesBlockingSimple,
			streaming: false,
			path:      pathOpenAIResponses,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_streaming_with_reasoning",
			fixture:   fixtures.OaiResponsesStreamingMultiReasoningBuiltinTool,
			streaming: true,
			path:      pathOpenAIResponses,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.RecordToolUsage", 1, codes.Unset},
				{"Intercept.RecordModelThought", 2, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_blocking_with_reasoning",
			fixture:   fixtures.OaiResponsesBlockingMultiReasoningBuiltinTool,
			streaming: false,
			path:      pathOpenAIResponses,
			expect: []expectTrace{
				{"Intercept", 1, codes.Unset},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Unset},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.RecordTokenUsage", 1, codes.Unset},
				{"Intercept.RecordToolUsage", 1, codes.Unset},
				{"Intercept.RecordModelThought", 2, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			sr, tracer := setupTracer(t)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withTracer(tracer),
			)

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			bridgeServer.Close()

			require.Equal(t, 1, len(bridgeServer.Recorder.RecordedInterceptions()))
			intcID := bridgeServer.Recorder.RecordedInterceptions()[0].ID

			totalCount := 0
			for _, e := range tc.expect {
				totalCount += e.count
			}
			require.Len(t, sr.Ended(), totalCount)

			attrs := []attribute.KeyValue{
				attribute.String(tracing.RequestPath, tc.path),
				attribute.String(tracing.InterceptionID, intcID),
				attribute.String(tracing.Provider, config.ProviderOpenAI),
				attribute.String(tracing.Model, gjson.Get(string(reqBody), "model").Str),
				attribute.String(tracing.InitiatorID, defaultActorID),
				attribute.Bool(tracing.Streaming, tc.streaming),
			}
			verifyTraces(t, sr, tc.expect, attrs)
		})
	}
}

func TestTraceOpenAIErr(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		fixture       []byte
		streaming     bool
		allowOverflow bool
		path          string

		expect     []expectTrace
		expectCode int
	}{
		{
			name:       "trace_openai_chat_streaming_error",
			fixture:    fixtures.OaiChatMidStreamError,
			streaming:  true,
			path:       pathOpenAIChatCompletions,
			expectCode: http.StatusOK,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:       "trace_openai_chat_blocking_error",
			fixture:    fixtures.OaiChatNonStreamError,
			streaming:  false,
			path:       pathOpenAIChatCompletions,
			expectCode: http.StatusBadRequest,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Error},
			},
		},
		{
			name:       "trace_openai_responses_streaming_error",
			streaming:  true,
			fixture:    fixtures.OaiResponsesStreamingWrongResponseFormat,
			path:       pathOpenAIResponses,
			expectCode: http.StatusOK,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.RecordPromptUsage", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_blocking_error",
			fixture:   fixtures.OaiResponsesBlockingWrongResponseFormat,
			streaming: false,
			path:      pathOpenAIResponses,
			// Fixture returns http 200 response with wrong body
			// responses forward received response as is so
			// expected code == 200 even though ProcessRequest
			// traces are expected to have error status
			expectCode: http.StatusOK,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Error},
			},
		},
		{
			name:          "trace_openai_responses_streaming_http_error",
			fixture:       fixtures.OaiResponsesStreamingHTTPErr,
			streaming:     true,
			allowOverflow: true, // 429 error causes retries

			path:       pathOpenAIResponses,
			expectCode: http.StatusTooManyRequests,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Unset},
			},
		},
		{
			name:      "trace_openai_responses_blocking_http_error",
			fixture:   fixtures.OaiResponsesBlockingHTTPErr,
			streaming: false,

			path:       pathOpenAIResponses,
			expectCode: http.StatusUnauthorized,
			expect: []expectTrace{
				{"Intercept", 1, codes.Error},
				{"Intercept.CreateInterceptor", 1, codes.Unset},
				{"Intercept.RecordInterception", 1, codes.Unset},
				{"Intercept.ProcessRequest", 1, codes.Error},
				{"Intercept.RecordInterceptionEnded", 1, codes.Unset},
				{"Intercept.ProcessRequest.Upstream", 1, codes.Error},
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			sr, tracer := setupTracer(t)

			fix := fixtures.Parse(t, tc.fixture)

			mockAPI := newMockUpstream(ctx, t, newFixtureResponse(fix))
			mockAPI.AllowOverflow = tc.allowOverflow
			bridgeServer := newBridgeTestServer(ctx, t, mockAPI.URL,
				withTracer(tracer),
			)

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.expectCode, resp.StatusCode)
			bridgeServer.Close()

			require.Equal(t, 1, len(bridgeServer.Recorder.RecordedInterceptions()))
			intcID := bridgeServer.Recorder.RecordedInterceptions()[0].ID

			totalCount := 0
			for _, e := range tc.expect {
				totalCount += e.count
			}
			require.Len(t, sr.Ended(), totalCount)

			attrs := []attribute.KeyValue{
				attribute.String(tracing.RequestPath, tc.path),
				attribute.String(tracing.InterceptionID, intcID),
				attribute.String(tracing.Provider, config.ProviderOpenAI),
				attribute.String(tracing.Model, gjson.Get(string(reqBody), "model").Str),
				attribute.String(tracing.InitiatorID, defaultActorID),
				attribute.Bool(tracing.Streaming, tc.streaming),
			}
			verifyTraces(t, sr, tc.expect, attrs)
		})
	}
}

func TestTracePassthrough(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.OaiChatFallthrough)

	upstream := newMockUpstream(t.Context(), t, newFixtureResponse(fix))

	sr, tracer := setupTracer(t)

	bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL,
		withTracer(tracer),
	)

	resp, err := bridgeServer.makeRequest(t, http.MethodGet, "/openai/v1/models", nil)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	bridgeServer.Close()

	spans := sr.Ended()
	require.Len(t, spans, 1)

	assert.Equal(t, spans[0].Name(), "Passthrough")
	want := []attribute.KeyValue{
		attribute.String(tracing.PassthroughMethod, "GET"),
		attribute.String(tracing.PassthroughUpstreamURL, upstream.URL+"/models"),
		attribute.String(tracing.PassthroughURL, "/models"),
	}
	got := slices.SortedFunc(slices.Values(spans[0].Attributes()), cmpAttrKeyVal)
	require.Equal(t, want, got)
}

func TestNewServerProxyManagerTraces(t *testing.T) {
	t.Parallel()

	sr, tracer := setupTracer(t)

	serverName := "serverName"
	mockMCP := setupMCPForTestWithName(t, serverName, tracer)
	tool := mockMCP.ListTools()[0]

	require.Len(t, sr.Ended(), 3)
	verifyTraces(t, sr, []expectTrace{{"ServerProxyManager.Init", 1, codes.Unset}}, []attribute.KeyValue{})

	attrs := []attribute.KeyValue{
		attribute.String(tracing.MCPProxyName, serverName),
		attribute.String(tracing.MCPServerURL, tool.ServerURL),
		attribute.String(tracing.MCPServerName, serverName),
	}
	verifyTraces(t, sr, []expectTrace{{"StreamableHTTPServerProxy.Init", 1, codes.Unset}}, attrs)

	attrs = append(attrs, attribute.Int(tracing.MCPToolCount, len(mockMCP.ListTools())))
	verifyTraces(t, sr, []expectTrace{{"StreamableHTTPServerProxy.Init.fetchTools", 1, codes.Unset}}, attrs)
}

func cmpAttrKeyVal(a attribute.KeyValue, b attribute.KeyValue) int {
	return strings.Compare(string(a.Key), string(b.Key))
}

// checks counts of traces with given name, status and attributes
func verifyTraces(t *testing.T, spanRecorder *tracetest.SpanRecorder, expect []expectTrace, attrs []attribute.KeyValue) {
	spans := spanRecorder.Ended()

	for _, e := range expect {
		found := 0
		for _, s := range spans {
			if s.Name() != e.name || s.Status().Code != e.status {
				continue
			}
			found++
			want := slices.SortedFunc(slices.Values(attrs), cmpAttrKeyVal)
			got := slices.SortedFunc(slices.Values(s.Attributes()), cmpAttrKeyVal)
			require.Equal(t, want, got)
			assert.Equalf(t, e.status, s.Status().Code, "unexpected status for trace naned: %v got: %v want: %v", e.name, s.Status().Code, e.status)
		}
		if found != e.count {
			t.Errorf("found unexpected number of spans named: %v with status %v, got: %v want: %v", e.name, e.status, found, e.count)
		}
	}
}
