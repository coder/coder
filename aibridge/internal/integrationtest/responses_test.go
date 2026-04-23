package integrationtest //nolint:testpackage // tests unexported internals

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/sjson"

	"github.com/coder/coder/v2/aibridge"
	"github.com/coder/coder/v2/aibridge/config"
	"github.com/coder/coder/v2/aibridge/fixtures"
	"github.com/coder/coder/v2/aibridge/internal/testutil"
	"github.com/coder/coder/v2/aibridge/provider"
	"github.com/coder/coder/v2/aibridge/recorder"
	"github.com/coder/coder/v2/aibridge/utils"
)

type keyVal struct {
	key string
	val any
}

func TestResponsesOutputMatchesUpstream(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                 string
		fixture              []byte
		streaming            bool
		expectModel          string
		expectPromptRecorded string
		expectToolRecorded   *recorder.ToolUsageRecord
		expectTokenUsage     *recorder.TokenUsageRecord
		userAgent            string
		expectedClient       aibridge.Client
	}{
		{
			name:                 "blocking_simple",
			fixture:              fixtures.OaiResponsesBlockingSimple,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "tell me a joke",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0388c79043df3e3400695f9f83cd6481959062cec6830d8d51",
				Input:  11,
				Output: 18,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     29,
				},
			},
			userAgent:      "claude-cli/2.0.67 (external, cli)",
			expectedClient: aibridge.ClientClaudeCode,
		},
		{
			name:                 "blocking_builtin_tool",
			fixture:              fixtures.OaiResponsesBlockingSingleBuiltinTool,
			expectModel:          "gpt-4.1",
			expectPromptRecorded: "Is 3 + 5 a prime number? Use the add function to calculate the sum.",
			expectToolRecorded: &recorder.ToolUsageRecord{
				MsgID:      "resp_0da6045a8b68fa5200695fa23dcc2c81a19c849f627abf8a31",
				Tool:       "add",
				ToolCallID: "call_CJSaa2u51JG996575oVljuNq",
				Args:       map[string]any{"a": float64(3), "b": float64(5)},
				Injected:   false,
			},
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0da6045a8b68fa5200695fa23dcc2c81a19c849f627abf8a31",
				Input:  58,
				Output: 18,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     76,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "blocking_cached_input_tokens",
			fixture:              fixtures.OaiResponsesBlockingCachedInputTokens,
			expectModel:          "gpt-4.1",
			expectPromptRecorded: "This was a large input...",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:                "resp_0cd5d6b8310055d600696a1776b42c81a199fbb02248a8bfa0",
				Input:                129, // 12033 input - 11904 cached
				Output:               44,
				CacheReadInputTokens: 11904,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     11904,
					"output_reasoning": 0,
					"total_tokens":     12077,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "blocking_custom_tool",
			fixture:              fixtures.OaiResponsesBlockingCustomTool,
			expectModel:          "gpt-5",
			expectPromptRecorded: "Use the code_exec tool to print hello world to the console.",
			expectToolRecorded: &recorder.ToolUsageRecord{
				MsgID:      "resp_09c614364030cdf000696942589da081a0af07f5859acb7308",
				Tool:       "code_exec",
				ToolCallID: "call_haf8njtwrVZ1754Gm6fjAtuA",
				Args:       "print(\"hello world\")",
				Injected:   false,
			},
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_09c614364030cdf000696942589da081a0af07f5859acb7308",
				Input:  64,
				Output: 148,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 128,
					"total_tokens":     212,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "blocking_conversation",
			fixture:              fixtures.OaiResponsesBlockingConversation,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "explain why this is funny.",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0c9f1f0524a858fa00695fa15fc5a081958f4304aafd3bdec2",
				Input:  48,
				Output: 116,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     164,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "blocking_prev_response_id",
			fixture:              fixtures.OaiResponsesBlockingPrevResponseID,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "explain why this is funny.",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0388c79043df3e3400695f9f86cfa08195af1f015c60117a83",
				Input:  43,
				Output: 129,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     172,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_simple",
			fixture:              fixtures.OaiResponsesStreamingSimple,
			streaming:            true,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "tell me a joke",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0f9c4b2f224d858000695fa062bf048197a680f357bbb09000",
				Input:  11,
				Output: 18,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     29,
				},
			},
			userAgent:      "Zed/0.219.4+stable.119.abc123 (macos; aarch64)",
			expectedClient: aibridge.ClientZed,
		},
		{
			name:                 "streaming_codex",
			fixture:              fixtures.OaiResponsesStreamingCodex,
			streaming:            true,
			expectModel:          "gpt-5-codex",
			expectPromptRecorded: "hello",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0e172b76542a9100016964f7e63d888191a2a28cb2ba0ab6d3",
				Input:  4006,
				Output: 13,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     4019,
				},
			},
			userAgent:      "codex_cli_rs/0.87.0 (Mac OS 26.2.0; arm64)",
			expectedClient: aibridge.ClientCodex,
		},
		{
			name:                 "streaming_builtin_tool",
			fixture:              fixtures.OaiResponsesStreamingBuiltinTool,
			streaming:            true,
			expectModel:          "gpt-4.1",
			expectPromptRecorded: "Is 3 + 5 a prime number? Use the add function to calculate the sum.",
			expectToolRecorded: &recorder.ToolUsageRecord{
				MsgID:      "resp_0c3fb28cfcf463a500695fa2f0239481a095ec6ce3dfe4d458",
				Tool:       "add",
				ToolCallID: "call_7VaiUXZYuuuwWwviCrckxq6t",
				Args:       map[string]any{"a": float64(3), "b": float64(5)},
				Injected:   false,
			},
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0c3fb28cfcf463a500695fa2f0239481a095ec6ce3dfe4d458",
				Input:  58,
				Output: 18,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     76,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_cached_tokens",
			fixture:              fixtures.OaiResponsesStreamingCachedInputTokens,
			streaming:            true,
			expectModel:          "gpt-5.2-codex",
			expectPromptRecorded: "Test cached input tokens.",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:                "resp_05080461b406f3f501696a1409d34c8195a40ff4b092145c35",
				Input:                1165, // 16909 input - 15744 cached
				Output:               54,
				CacheReadInputTokens: 15744,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     15744,
					"output_reasoning": 0,
					"total_tokens":     16963,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_custom_tool",
			fixture:              fixtures.OaiResponsesStreamingCustomTool,
			streaming:            true,
			expectModel:          "gpt-5",
			expectPromptRecorded: "Use the code_exec tool to print hello world to the console.",
			expectToolRecorded: &recorder.ToolUsageRecord{
				MsgID:      "resp_0c26996bc41c2a0500696942e83634819fb71b2b8ff8a4a76c",
				Tool:       "code_exec",
				ToolCallID: "call_2gSnF58IEhXLwlbnqbm5XKMd",
				Args:       "print(\"hello world\")",
				Injected:   false,
			},
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0c26996bc41c2a0500696942e83634819fb71b2b8ff8a4a76c",
				Input:  64,
				Output: 340,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 320,
					"total_tokens":     404,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_conversation",
			fixture:              fixtures.OaiResponsesStreamingConversation,
			streaming:            true,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "explain why this is funny.",
			expectedClient:       aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_prev_response_id",
			fixture:              fixtures.OaiResponsesStreamingPrevResponseID,
			streaming:            true,
			expectModel:          "gpt-4o-mini",
			expectPromptRecorded: "explain why this is funny.",
			expectTokenUsage: &recorder.TokenUsageRecord{
				MsgID:  "resp_0f9c4b2f224d858000695fa0649b8c8197b38914b15a7add0e",
				Input:  43,
				Output: 182,
				ExtraTokenTypes: map[string]int64{
					"input_cached":     0,
					"output_reasoning": 0,
					"total_tokens":     225,
				},
			},
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "stream_error",
			fixture:              fixtures.OaiResponsesStreamingStreamError,
			streaming:            true,
			expectModel:          "gpt-6.7",
			expectPromptRecorded: "hello_stream_error",
			expectedClient:       aibridge.ClientUnknown,
		},
		{
			name:                 "stream_failure",
			fixture:              fixtures.OaiResponsesStreamingStreamFailure,
			streaming:            true,
			expectModel:          "gpt-6.7",
			expectPromptRecorded: "hello_stream_failure",
			expectedClient:       aibridge.ClientUnknown,
		},

		// Original status code and body is kept even with wrong json format
		{
			name:           "blocking_wrong_format",
			fixture:        fixtures.OaiResponsesBlockingWrongResponseFormat,
			expectModel:    "gpt-6.7",
			expectedClient: aibridge.ClientUnknown,
		},
		{
			name:                 "streaming_wrong_format",
			fixture:              fixtures.OaiResponsesStreamingWrongResponseFormat,
			streaming:            true,
			expectModel:          "gpt-6.7",
			expectPromptRecorded: "hello_wrong_format",
			expectedClient:       aibridge.ClientUnknown,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request(), http.Header{"User-Agent": {tc.userAgent}})
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			got, err := io.ReadAll(resp.Body)

			require.NoError(t, err)
			if tc.streaming {
				require.Equal(t, string(fix.Streaming()), string(got))
			} else {
				require.Equal(t, string(fix.NonStreaming()), string(got))
			}

			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1)
			intc := interceptions[0]
			require.Equal(t, intc.InitiatorID, defaultActorID)
			require.Equal(t, intc.Provider, config.ProviderOpenAI)
			require.Equal(t, intc.Model, tc.expectModel)
			require.Equal(t, tc.userAgent, intc.UserAgent)
			require.Equal(t, string(tc.expectedClient), intc.Client)

			recordedPrompts := bridgeServer.Recorder.RecordedPromptUsages()
			if tc.expectPromptRecorded != "" {
				require.Len(t, recordedPrompts, 1)
				promptEq := func(pur *recorder.PromptUsageRecord) bool { return pur.Prompt == tc.expectPromptRecorded }
				require.Truef(t, slices.ContainsFunc(recordedPrompts, promptEq), "promnt not found, got: %v, want: %v", recordedPrompts, tc.expectPromptRecorded)
			} else {
				require.Empty(t, recordedPrompts)
			}

			recordedTools := bridgeServer.Recorder.RecordedToolUsages()
			if tc.expectToolRecorded != nil {
				require.Len(t, recordedTools, 1)
				recordedTools[0].InterceptionID = tc.expectToolRecorded.InterceptionID // ignore interception id (interception id is not constant and response doesn't contain it)
				recordedTools[0].CreatedAt = tc.expectToolRecorded.CreatedAt           // ignore time
				require.Equal(t, tc.expectToolRecorded, recordedTools[0])
			} else {
				require.Empty(t, recordedTools)
			}

			recordedTokens := bridgeServer.Recorder.RecordedTokenUsages()
			if tc.expectTokenUsage != nil {
				require.Len(t, recordedTokens, 1)
				recordedTokens[0].InterceptionID = tc.expectTokenUsage.InterceptionID // ignore interception id
				recordedTokens[0].CreatedAt = tc.expectTokenUsage.CreatedAt           // ignore time
				require.Equal(t, tc.expectTokenUsage, recordedTokens[0])
			} else {
				require.Empty(t, recordedTokens)
			}
		})
	}
}

func TestResponsesBackgroundModeForbidden(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		streaming bool
	}{
		{
			name:      "blocking",
			streaming: false,
		},
		{
			name:      "streaming",
			streaming: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// request with Background mode should be rejected before it reaches upstream
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				t.Errorf("unexpected request to upstream: %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			t.Cleanup(upstream.Close)

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			// Create a request with background mode enabled
			reqBytes := responsesRequestBytes(t, tc.streaming, keyVal{"background", true})
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, reqBytes)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, http.StatusNotImplemented, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			requireResponsesError(t, http.StatusNotImplemented, "background requests are currently not supported by AI Bridge", body)
		})
	}
}

func TestResponsesParallelToolsOverwritten(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		fixture           [2][]byte // [blocking, streaming] fixture pair.
		withInjectedTools bool
		initialSetting    *bool
		expectedSetting   *bool // nil = field should not be present, non-nil = expected value.
	}{
		// With injected tools and builtin tools: parallel_tool_calls should be forced false.
		{
			name:              "with injected and builtin tools: parallel_tool_calls true",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected and builtin tools: parallel_tool_calls false",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected and builtin tools: parallel_tool_calls unset",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: true,
			initialSetting:    nil,
			expectedSetting:   utils.PtrTo(false),
		},
		// With injected tools but without builtin tools: parallel_tool_calls should be forced false.
		{
			name:              "with injected tools only: parallel_tool_calls true",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected tools only: parallel_tool_calls false",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected tools only: parallel_tool_calls unset",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: true,
			initialSetting:    nil,
			expectedSetting:   utils.PtrTo(false),
		},
		// With builtin tools but without injected tools: parallel_tool_calls should be preserved.
		{
			name:              "with builtin tools only: parallel_tool_calls true",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(true),
		},
		{
			name:              "with builtin tools only: parallel_tool_calls false",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with builtin tools only: parallel_tool_calls unset",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSingleBuiltinTool, fixtures.OaiResponsesStreamingBuiltinTool},
			withInjectedTools: false,
			initialSetting:    nil,
			expectedSetting:   nil,
		},
		// Without any tools: nothing is modified.
		{
			name:              "no tools: parallel_tool_calls true",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(true),
		},
		{
			name:              "no tools: parallel_tool_calls false",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "no tools: parallel_tool_calls unset",
			fixture:           [2][]byte{fixtures.OaiResponsesBlockingSimple, fixtures.OaiResponsesStreamingSimple},
			withInjectedTools: false,
			initialSetting:    nil,
			expectedSetting:   nil,
		},
	}

	for _, tc := range cases {
		for i, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/streaming=%v", tc.name, streaming), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, tc.fixture[i])
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

				var opts []bridgeOption
				if tc.withInjectedTools {
					opts = append(opts, withMCP(setupMCPForTest(t, defaultTracer)))
				}
				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, opts...)

				var (
					reqBody = fix.Request()
					err     error
				)
				if tc.initialSetting != nil {
					reqBody, err = sjson.SetBytes(reqBody, "parallel_tool_calls", *tc.initialSetting)
					require.NoError(t, err)
				}

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				received := upstream.receivedRequests()
				require.Len(t, received, 1)

				var upstreamReq map[string]any
				require.NoError(t, json.Unmarshal(received[0].Body, &upstreamReq))

				ptc, ok := upstreamReq["parallel_tool_calls"].(bool)
				require.Equal(t, tc.expectedSetting != nil, ok,
					"parallel_tool_calls presence mismatch")
				if tc.expectedSetting != nil {
					assert.Equal(t, *tc.expectedSetting, ptc)
				}
			})
		}
	}
}

func TestClientAndConnectionError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		addr        string
		streaming   bool
		errContains string
	}{
		{
			name:        "blocking_connection_refused",
			addr:        startRejectingListener(t),
			streaming:   false,
			errContains: "connection reset by peer",
		},
		{
			name:        "streaming_connection_refused",
			addr:        startRejectingListener(t),
			streaming:   true,
			errContains: "connection reset by peer",
		},
		{
			name:        "blocking_bad_url",
			addr:        "not_url",
			streaming:   false,
			errContains: "unsupported protocol scheme",
		},
		{
			name:        "streaming_bad_url",
			addr:        "not_url",
			streaming:   true,
			errContains: "unsupported protocol scheme",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// tc.addr may be an intentionally invalid URL; use withCustomProvider.
			// MaxRetries is set to 0 to disable SDK retries and speed up the test.
			cfg := openAICfg(tc.addr, apiKey)
			maxRetries := 0
			cfg.MaxRetries = &maxRetries
			bridgeServer := newBridgeTestServer(ctx, t, tc.addr, withCustomProvider(provider.NewOpenAI(cfg)))

			reqBytes := responsesRequestBytes(t, tc.streaming)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, reqBytes)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, "application/json", resp.Header.Get("Content-Type"))
			require.Equal(t, http.StatusInternalServerError, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			requireResponsesError(t, http.StatusInternalServerError, tc.errContains, body)
			require.Empty(t, bridgeServer.Recorder.RecordedPromptUsages())
		})
	}
}

func TestUpstreamError(t *testing.T) {
	t.Parallel()

	responsesError := `{"error":{"message":"Something went wrong","type":"invalid_request_error","param":null,"code":"invalid_request"}}`
	nonResponsesError := `plain text error`

	tests := []struct {
		name        string
		streaming   bool
		statusCode  int
		contentType string
		body        string
	}{
		{
			name:        "blocking_responses_error",
			streaming:   false,
			statusCode:  http.StatusBadRequest,
			contentType: "application/json",
			body:        responsesError,
		},
		{
			name:        "streaming_responses_error",
			streaming:   true,
			statusCode:  http.StatusBadRequest,
			contentType: "application/json",
			body:        responsesError,
		},
		{
			name:        "blocking_non_responses_error",
			streaming:   false,
			statusCode:  http.StatusBadGateway,
			contentType: "text/plain",
			body:        nonResponsesError,
		},
		{
			name:        "streaming_non_responses_error",
			streaming:   true,
			statusCode:  http.StatusBadGateway,
			contentType: "text/plain",
			body:        nonResponsesError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.statusCode)
				_, err := w.Write([]byte(tc.body))
				require.NoError(t, err)
			}))
			t.Cleanup(upstream.Close)

			// MaxRetries is set to 0 to disable SDK retries and speed up the test.
			cfg := openAICfg(upstream.URL, apiKey)
			maxRetries := 0
			cfg.MaxRetries = &maxRetries
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withCustomProvider(provider.NewOpenAI(cfg)))

			reqBytes := responsesRequestBytes(t, tc.streaming)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, reqBytes)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, tc.statusCode, resp.StatusCode)
			require.Equal(t, tc.contentType, resp.Header.Get("Content-Type"))

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, tc.body, string(body))
		})
	}
}

// TestResponsesInjectedTool tests that injected MCP tool calls trigger the inner agentic loop,
// invoke the tool via MCP, and send the result back to the model.
func TestResponsesInjectedTool(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name              string
		fixture           []byte
		streaming         bool
		mcpToolName       string
		expectToolArgs    map[string]any
		expectToolError   string // If non-empty, MCP tool returns this error.
		expectPrompt      string
		expectTokenUsages []recorder.TokenUsageRecord
	}{
		{
			name:        "blocking_success",
			fixture:     fixtures.OaiResponsesBlockingSingleInjectedTool,
			mcpToolName: "coder_template_version_parameters",
			expectToolArgs: map[string]any{
				"template_version_id": "aa4e30e4-a086-4df6-a364-1343f1458104",
			},
			expectPrompt: "list the template params for version aa4e30e4-a086-4df6-a364-1343f1458104",
			expectTokenUsages: []recorder.TokenUsageRecord{
				{
					MsgID:                "resp_012db006225b0ec700696b5de8a01481a28182ea6885448f93",
					Input:                227, // 6371 input - 6144 cached
					Output:               75,
					CacheReadInputTokens: 6144,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     6144,
						"output_reasoning": 25,
						"total_tokens":     6446,
					},
				},
				{
					MsgID:                "resp_012db006225b0ec700696b5dec1d4c81a2a6a416e31af39b90",
					Input:                612, // 6756 input - 6144 cached
					Output:               231,
					CacheReadInputTokens: 6144,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     6144,
						"output_reasoning": 43,
						"total_tokens":     6987,
					},
				},
			},
		},
		{
			name:        "blocking_tool_error",
			fixture:     fixtures.OaiResponsesBlockingSingleInjectedToolError,
			mcpToolName: "coder_delete_template",
			expectToolArgs: map[string]any{
				"template_id": "03cb4fdd-8109-4a22-8e22-bb4975171395",
			},
			expectPrompt:    "delete the template with ID 03cb4fdd-8109-4a22-8e22-bb4975171395, don't ask for confirmation",
			expectToolError: "500 Internal error deleting template: unauthorized: rbac: forbidden",
			expectTokenUsages: []recorder.TokenUsageRecord{
				{
					MsgID:                "resp_06e2afba24b6b2ad00696b774d1df0819eaf1ec802bc8a2ca9",
					Input:                233, // 6377 input - 6144 cached
					Output:               119,
					CacheReadInputTokens: 6144,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     6144,
						"output_reasoning": 70,
						"total_tokens":     6496,
					},
				},
				{
					MsgID:                "resp_06e2afba24b6b2ad00696b775044e8819ea14840698ef966e2",
					Input:                395, // 6539 input - 6144 cached
					Output:               144,
					CacheReadInputTokens: 6144,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     6144,
						"output_reasoning": 28,
						"total_tokens":     6683,
					},
				},
			},
		},
		{
			name:           "streaming_success",
			fixture:        fixtures.OaiResponsesStreamingSingleInjectedTool,
			streaming:      true,
			mcpToolName:    "coder_list_templates",
			expectToolArgs: map[string]any{},
			expectPrompt:   "List my coder templates.",
			expectTokenUsages: []recorder.TokenUsageRecord{
				{
					MsgID:  "resp_016595fe42aa62ca0069724419c52081a0b7eb479c6bc8109f",
					Input:  6269, // 6269 input - 0 cached
					Output: 18,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     0,
						"output_reasoning": 0,
						"total_tokens":     6287,
					},
				},
				{
					MsgID:                "resp_0bc5f54fce6df69a006972442175908194bb81d31f576e6ca6",
					Input:                319, // 6463 input - 6144 cached
					Output:               182,
					CacheReadInputTokens: 6144,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     6144,
						"output_reasoning": 0,
						"total_tokens":     6645,
					},
				},
			},
		},
		{
			name:        "streaming_tool_error",
			fixture:     fixtures.OaiResponsesStreamingSingleInjectedToolError,
			streaming:   true,
			mcpToolName: "coder_create_workspace_build",
			expectToolArgs: map[string]any{
				"transition":   "start",
				"workspace_id": "non_existing_id",
			},
			expectPrompt:    "Create a new workspace build for an workspace with id: 'non_existing_id'",
			expectToolError: "workspace_id must be a valid UUID: invalid UUID length: 15",
			expectTokenUsages: []recorder.TokenUsageRecord{
				{
					MsgID:  "resp_0dfed48e1052ad7f0069725ca129f88193b97d6deff1760524",
					Input:  6280, // 6280 input - 0 cached
					Output: 30,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     0,
						"output_reasoning": 0,
						"total_tokens":     6310,
					},
				},
				{
					MsgID:  "resp_0dfed48e1052ad7f0069725ca39880819390fcc5b2eb8cf8c6",
					Input:  6346, // 6346 input - 0 cached
					Output: 56,
					ExtraTokenTypes: map[string]int64{
						"input_cached":     0,
						"output_reasoning": 0,
						"total_tokens":     6402,
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Setup mock server for multi-turn interaction.
			// First request → tool call response, second → tool response.
			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix), newFixtureToolResponse(fix))

			// Setup MCP server proxies (with mock tools).
			mockMCP := setupMCPForTest(t, defaultTracer)
			if tc.expectToolError != "" {
				mockMCP.setToolError(tc.mcpToolName, tc.expectToolError)
			}

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withMCP(mockMCP))

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request())
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			body, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Wait for both requests to be made (inner agentic loop).
			require.Eventually(t, func() bool {
				return upstream.Calls.Load() == 2
			}, testutil.WaitMedium, testutil.IntervalFast)

			// Verify the injected tool was invoked via MCP.
			invocations := mockMCP.getCallsByTool(tc.mcpToolName)
			require.Len(t, invocations, 1, "expected MCP tool to be invoked once")

			// Verify the injected tool usage was recorded.
			toolUsages := bridgeServer.Recorder.RecordedToolUsages()
			require.Len(t, toolUsages, 1)
			require.Equal(t, tc.mcpToolName, toolUsages[0].Tool)
			require.Equal(t, tc.expectToolArgs, toolUsages[0].Args)
			require.True(t, toolUsages[0].Injected, "injected tool should be marked as injected")
			if tc.expectToolError != "" {
				require.Contains(t, toolUsages[0].InvocationError.Error(), tc.expectToolError)
			}

			// Verify prompt was recorded.
			prompts := bridgeServer.Recorder.RecordedPromptUsages()
			require.Len(t, prompts, 1)
			require.Equal(t, tc.expectPrompt, prompts[0].Prompt)

			tokenUsages := bridgeServer.Recorder.RecordedTokenUsages()
			require.Len(t, tokenUsages, len(tc.expectTokenUsages))
			for i := range tokenUsages {
				tokenUsages[i].InterceptionID = "" // ignore interception ID and time creation when comparing
				tokenUsages[i].CreatedAt = time.Time{}
				require.Equal(t, tc.expectTokenUsages[i], *tokenUsages[i])
			}

			// Verify the response is the final tool response (after agentic loop).
			if tc.streaming {
				require.Equal(t, string(fix.StreamingToolCall()), string(body))
			} else {
				require.Equal(t, string(fix.NonStreamingToolCall()), string(body))
			}
		})
	}
}

func TestResponsesModelThoughts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		fixture          []byte
		expectedThoughts []recorder.ModelThoughtRecord // nil means no tool usages expected at all
	}{
		{
			name:             "single reasoning/blocking",
			fixture:          fixtures.OaiResponsesBlockingSingleBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants to add 3 and 5", recorder.ThoughtSourceReasoningSummary)},
		},
		{
			name:             "single reasoning/streaming",
			fixture:          fixtures.OaiResponsesStreamingBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants to add 3 and 5", recorder.ThoughtSourceReasoningSummary)},
		},
		{
			name:    "multiple reasoning items/blocking",
			fixture: fixtures.OaiResponsesBlockingMultiReasoningBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("The user wants to add 3 and 5", recorder.ThoughtSourceReasoningSummary),
				newModelThought("After adding, I will check if the result is prime", recorder.ThoughtSourceReasoningSummary),
			},
		},
		{
			name:    "multiple reasoning items/streaming",
			fixture: fixtures.OaiResponsesStreamingMultiReasoningBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("The user wants to add 3 and 5", recorder.ThoughtSourceReasoningSummary),
				newModelThought("After adding, I will check if the result is prime", recorder.ThoughtSourceReasoningSummary),
			},
		},
		{
			name:             "commentary/blocking",
			fixture:          fixtures.OaiResponsesBlockingCommentaryBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("Checking whether 3 + 5 is prime by calling the add function first.", recorder.ThoughtSourceCommentary)},
		},
		{
			name:             "commentary/streaming",
			fixture:          fixtures.OaiResponsesStreamingCommentaryBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("Checking whether 3 + 5 is prime by calling the add function first.", recorder.ThoughtSourceCommentary)},
		},
		{
			name:    "summary and commentary/blocking",
			fixture: fixtures.OaiResponsesBlockingSummaryAndCommentaryBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("I need to add 3 and 5 to check primality.", recorder.ThoughtSourceReasoningSummary),
				newModelThought("Let me calculate the sum first using the add function.", recorder.ThoughtSourceCommentary),
			},
		},
		{
			name:    "summary and commentary/streaming",
			fixture: fixtures.OaiResponsesStreamingSummaryAndCommentaryBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("I need to add 3 and 5 to check primality.", recorder.ThoughtSourceReasoningSummary),
				newModelThought("Let me calculate the sum first using the add function.", recorder.ThoughtSourceCommentary),
			},
		},
		{
			name:             "parallel tool calls/blocking",
			fixture:          fixtures.OaiResponsesBlockingSingleBuiltinToolParallel,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants two additions", recorder.ThoughtSourceReasoningSummary)},
		},
		{
			name:             "parallel tool calls/streaming",
			fixture:          fixtures.OaiResponsesStreamingSingleBuiltinToolParallel,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants two additions", recorder.ThoughtSourceReasoningSummary)},
		},
		{
			name:             "thoughts without tool calls",
			fixture:          fixtures.OaiResponsesStreamingCodex, // This fixture contains reasoning, but it's not associated with tool calls.
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("Preparing simple response", recorder.ThoughtSourceReasoningSummary)},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIResponses, fix.Request())
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			bridgeServer.Recorder.VerifyModelThoughtsRecorded(t, tc.expectedThoughts)
			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
		})
	}
}

func requireResponsesError(t *testing.T, code int, message string, body []byte) {
	var respErr responses.Error
	err := json.Unmarshal(body, &respErr)
	require.NoError(t, err)

	require.Equal(t, strconv.Itoa(code), respErr.Code)
	require.Contains(t, respErr.Message, message)
}

func responsesRequestBytes(t *testing.T, streaming bool, additionalFields ...keyVal) []byte {
	reqBody := map[string]any{
		"input":  "tell me a joke",
		"model":  "gpt-4o-mini",
		"stream": streaming,
	}

	for _, kv := range additionalFields {
		reqBody[kv.key] = kv.val
	}

	reqBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)
	return reqBytes
}

func startRejectingListener(t *testing.T) (addr string) {
	t.Helper()
	var wg sync.WaitGroup

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = ln.Close()
		wg.Wait()
	})

	go func() {
		for {
			wg.Add(1)
			defer wg.Done()

			c, err := ln.Accept()
			if err != nil {
				// When ln.Close() is called, Accept returns an error -> exit.
				return
			}

			// Read at least 1 byte so the client has started writing
			// before we RST, ensuring a consistent "connection reset by peer".
			buf := make([]byte, 1)
			_, _ = c.Read(buf)
			if tc, ok := c.(*net.TCPConn); ok {
				_ = tc.SetLinger(0)
			}
			_ = c.Close()
		}
	}()

	return "http://" + ln.Addr().String()
}
