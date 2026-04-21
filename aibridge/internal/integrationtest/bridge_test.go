package integrationtest //nolint:testpackage // tests unexported internals

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/ssestream"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
	"github.com/google/uuid"
	"github.com/openai/openai-go/v3"
	oaissestream "github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"go.uber.org/goleak"
	"golang.org/x/xerrors"

	"github.com/coder/aibridge"
	"github.com/coder/aibridge/config"
	"github.com/coder/aibridge/fixtures"
	"github.com/coder/aibridge/intercept"
	"github.com/coder/aibridge/internal/testutil"
	"github.com/coder/aibridge/mcp"
	"github.com/coder/aibridge/provider"
	"github.com/coder/aibridge/recorder"
	"github.com/coder/aibridge/utils"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestAnthropicMessages(t *testing.T) {
	t.Parallel()

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name                          string
			streaming                     bool
			expectedInputTokens           int
			expectedOutputTokens          int
			expectedCacheReadInputTokens  int
			expectedCacheWriteInputTokens int
			expectedToolCallID            string
		}{
			{
				name:                          "streaming",
				streaming:                     true,
				expectedInputTokens:           2,
				expectedOutputTokens:          66,
				expectedCacheReadInputTokens:  13993,
				expectedCacheWriteInputTokens: 22,
				expectedToolCallID:            "toolu_01RX68weRSquLx6HUTj65iBo",
			},
			{
				name:                          "non-streaming",
				streaming:                     false,
				expectedInputTokens:           5,
				expectedOutputTokens:          84,
				expectedCacheReadInputTokens:  23490,
				expectedCacheWriteInputTokens: 0,
				expectedToolCallID:            "toolu_01AusGgY5aKFhzWrFBv9JfHq",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

				// Make API call to aibridge for Anthropic /v1/messages
				reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
				require.NoError(t, err)
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)

				// Response-specific checks.
				if tc.streaming {
					sp := aibridge.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))

					// Ensure the message starts and completes, at a minimum.
					assert.Contains(t, sp.AllEvents(), "message_start")
					assert.Contains(t, sp.AllEvents(), "message_stop")
				}

				expectedTokenRecordings := 1
				if tc.streaming {
					// One for message_start, one for message_delta.
					expectedTokenRecordings = 2
				}
				tokenUsages := bridgeServer.Recorder.RecordedTokenUsages()
				require.Len(t, tokenUsages, expectedTokenRecordings)

				assert.EqualValues(t, tc.expectedInputTokens, bridgeServer.Recorder.TotalInputTokens(), "input tokens miscalculated")
				assert.EqualValues(t, tc.expectedOutputTokens, bridgeServer.Recorder.TotalOutputTokens(), "output tokens miscalculated")
				assert.EqualValues(t, tc.expectedCacheReadInputTokens, bridgeServer.Recorder.TotalCacheReadInputTokens(), "cache read input tokens miscalculated")
				assert.EqualValues(t, tc.expectedCacheWriteInputTokens, bridgeServer.Recorder.TotalCacheWriteInputTokens(), "cache write input tokens miscalculated")

				toolUsages := bridgeServer.Recorder.RecordedToolUsages()
				require.Len(t, toolUsages, 1)
				assert.Equal(t, "Read", toolUsages[0].Tool)
				assert.Equal(t, tc.expectedToolCallID, toolUsages[0].ToolCallID)
				require.IsType(t, json.RawMessage{}, toolUsages[0].Args)
				var args map[string]any
				require.NoError(t, json.Unmarshal(toolUsages[0].Args.(json.RawMessage), &args))
				require.Contains(t, args, "file_path")
				assert.Equal(t, "/tmp/blah/foo", args["file_path"])

				promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
				require.Len(t, promptUsages, 1)
				assert.Equal(t, "read the foo file", promptUsages[0].Prompt)

				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})
}

func TestAnthropicMessagesModelThoughts(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name             string
		streaming        bool
		fixture          []byte
		expectedThoughts []recorder.ModelThoughtRecord // nil means no model thoughts expected
	}{
		{
			name:             "single thinking block/streaming",
			streaming:        true,
			fixture:          fixtures.AntSingleBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants me to read", recorder.ThoughtSourceThinking)},
		},
		{
			name:             "single thinking block/blocking",
			streaming:        false,
			fixture:          fixtures.AntSingleBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants me to read", recorder.ThoughtSourceThinking)},
		},
		{
			name:      "multiple thinking blocks/streaming",
			streaming: true,
			fixture:   fixtures.AntMultiThinkingBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("The user wants me to read", recorder.ThoughtSourceThinking),
				newModelThought("I should use the Read tool", recorder.ThoughtSourceThinking),
			},
		},
		{
			name:      "multiple thinking blocks/blocking",
			streaming: false,
			fixture:   fixtures.AntMultiThinkingBuiltinTool,
			expectedThoughts: []recorder.ModelThoughtRecord{
				newModelThought("The user wants me to read", recorder.ThoughtSourceThinking),
				newModelThought("I should use the Read tool", recorder.ThoughtSourceThinking),
			},
		},
		{
			name:             "parallel tool calls/streaming",
			streaming:        true,
			fixture:          fixtures.AntSingleBuiltinToolParallel,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants me to read two files", recorder.ThoughtSourceThinking)},
		},
		{
			name:             "parallel tool calls/blocking",
			streaming:        false,
			fixture:          fixtures.AntSingleBuiltinToolParallel,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("The user wants me to read two files", recorder.ThoughtSourceThinking)},
		},
		{
			name:             "thoughts without tool calls/streaming",
			streaming:        true,
			fixture:          fixtures.AntSimple,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("This is a classic philosophical question about medieval scholasticism", recorder.ThoughtSourceThinking)},
		},
		{
			name:             "thoughts without tool calls/blocking",
			streaming:        false,
			fixture:          fixtures.AntSimple,
			expectedThoughts: []recorder.ModelThoughtRecord{newModelThought("This is a classic philosophical question about medieval scholasticism", recorder.ThoughtSourceThinking)},
		},
		{
			name:             "no thoughts captured",
			streaming:        false,
			fixture:          fixtures.AntSingleInjectedTool,
			expectedThoughts: nil,
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

			reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
			require.NoError(t, err)
			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			if tc.streaming {
				sp := aibridge.NewSSEParser()
				require.NoError(t, sp.Parse(resp.Body))
				assert.Contains(t, sp.AllEvents(), "message_start")
				assert.Contains(t, sp.AllEvents(), "message_stop")
			}

			bridgeServer.Recorder.VerifyModelThoughtsRecorded(t, tc.expectedThoughts)
			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
		})
	}
}

func TestAWSBedrockIntegration(t *testing.T) {
	t.Parallel()

	t.Run("invalid config", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
		t.Cleanup(cancel)

		// Invalid bedrock config - missing region & base url
		bedrockCfg := &config.AWSBedrock{
			Region:          "",
			AccessKey:       "test-key",
			AccessKeySecret: "test-secret",
			Model:           "test-model",
			SmallFastModel:  "test-haiku",
		}

		bridgeServer := newBridgeTestServer(ctx, t, "http://unused",
			withCustomProvider(provider.NewAnthropic(anthropicCfg("http://unused", apiKey), bedrockCfg)),
		)

		resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, fixtures.Request(t, fixtures.AntSingleBuiltinTool))
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusInternalServerError, resp.StatusCode)
		body, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Contains(t, string(body), "create anthropic client")
		require.Contains(t, string(body), "region or base url required")
	})

	t.Run("/v1/messages", func(t *testing.T) {
		for _, streaming := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/streaming=%v", t.Name(), streaming), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.AntSingleBuiltinTool)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

				// We define region here to validate that with Region & BaseURL defined, the latter takes precedence.
				bedrockCfg := &config.AWSBedrock{
					Region:          "us-west-2",
					AccessKey:       "test-access-key",
					AccessKeySecret: "test-secret-key",
					Model:           "danthropic",      // This model should override the request's given one.
					SmallFastModel:  "danthropic-mini", // Unused but needed for validation.
					BaseURL:         upstream.URL,      // Use the mock server.
				}

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
					withCustomProvider(provider.NewAnthropic(anthropicCfg(upstream.URL, apiKey), bedrockCfg)),
				)

				// Make API call to aibridge for Anthropic /v1/messages, which will be routed via AWS Bedrock.
				// We override the AWS Bedrock client to route requests through our mock server.
				reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
				require.NoError(t, err)
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()

				// For streaming responses, consume the body to allow the stream to complete.
				if streaming {
					// Read the streaming response.
					_, err = io.ReadAll(resp.Body)
					require.NoError(t, err)
				}

				// Verify that Bedrock-specific model name was used in the request to the mock server
				// and the interception data.
				received := upstream.receivedRequests()
				require.Len(t, received, 1)

				// The Anthropic SDK's Bedrock middleware extracts "model" and "stream"
				// from the JSON body and encodes them in the URL path.
				// See: https://github.com/anthropics/anthropic-sdk-go/blob/4d669338f2041f3c60640b6dd317c4895dc71cd4/bedrock/bedrock.go#L247-L248
				pathParts := strings.Split(received[0].Path, "/")
				require.True(t, len(pathParts) >= 3 && pathParts[1] == "model", "unexpected path: %s", received[0].Path)
				require.Equal(t, bedrockCfg.Model, pathParts[2])
				require.False(t, gjson.GetBytes(received[0].Body, "model").Exists(), "model should be stripped from body")
				require.False(t, gjson.GetBytes(received[0].Body, "stream").Exists(), "stream should be stripped from body")

				interceptions := bridgeServer.Recorder.RecordedInterceptions()
				require.Len(t, interceptions, 1)
				require.Equal(t, interceptions[0].Model, bedrockCfg.Model)
				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})

	// Tests that Bedrock-incompatible fields are stripped and adaptive thinking
	// is handled correctly per model. Different Bedrock model names trigger
	// different behavior for beta flag filtering and field stripping.
	t.Run("unsupported fields removed", func(t *testing.T) {
		t.Parallel()

		// All fields in the fixture request that Bedrock may strip. Fields
		// listed in a test case's expectKeptFields survive; all others must
		// be absent from the forwarded body.
		strippableFields := []string{
			"metadata", "service_tier", "container", "inference_geo", // always stripped
			"output_config", "context_management", // stripped unless their beta flag survives
		}

		cases := []struct {
			name               string
			model              string
			smallFastModel     string
			expectThinkingType string
			expectBudgetTokens int64    // 0 means budget_tokens should not be present
			expectKeptFields   []string // fields from strippableFields expected to survive
			expectedBetaFlags  []string // values expected in the anthropic_beta array in the forwarded body
		}{
			// "beddel" matches no model prefix, so adaptive thinking is converted
			// to enabled with budget, and all model-gated beta flags are stripped.
			{
				name:               "beddel",
				model:              "beddel",
				smallFastModel:     "modrock",
				expectThinkingType: "enabled",
				expectBudgetTokens: 16000, // 32000 * 0.5 (medium effort)
				expectedBetaFlags:  []string{"interleaved-thinking-2025-05-14"},
			},
			// Opus 4.5 supports the effort beta, so output_config is kept.
			{
				name:               "opus-4.5",
				model:              "anthropic.claude-opus-4-5-20250514-v1:0",
				smallFastModel:     "anthropic.claude-haiku-4-5-20241022-v1:0",
				expectThinkingType: "enabled",
				expectBudgetTokens: 16000,
				expectKeptFields:   []string{"output_config"},
				expectedBetaFlags:  []string{"interleaved-thinking-2025-05-14", "effort-2025-11-24"},
			},
			// Sonnet 4.5 supports context-management beta, so context_management is kept.
			{
				name:               "sonnet-4.5",
				model:              "anthropic.claude-sonnet-4-5-20241022-v2:0",
				smallFastModel:     "anthropic.claude-haiku-4-5-20241022-v1:0",
				expectThinkingType: "enabled",
				expectBudgetTokens: 16000,
				expectKeptFields:   []string{"context_management"},
				expectedBetaFlags:  []string{"interleaved-thinking-2025-05-14", "context-management-2025-06-27"},
			},
			// Opus 4.6 supports adaptive thinking natively, so it is kept as-is.
			// Neither effort nor context-management betas apply to this model.
			{
				name:               "opus-4.6",
				model:              "anthropic.claude-opus-4-6-20260619-v1:0",
				smallFastModel:     "anthropic.claude-haiku-4-5-20241022-v1:0",
				expectThinkingType: "adaptive",
				expectedBetaFlags:  []string{"interleaved-thinking-2025-05-14"},
			},
		}

		for _, tc := range cases {
			for _, streaming := range []bool{true, false} {
				t.Run(fmt.Sprintf("%s/streaming=%v", tc.name, streaming), func(t *testing.T) {
					t.Parallel()

					ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
					t.Cleanup(cancel)

					fix := fixtures.Parse(t, fixtures.AntSimpleBedrock)
					upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

					bCfg := &config.AWSBedrock{
						Region:          "us-west-2",
						AccessKey:       "test-access-key",
						AccessKeySecret: "test-secret-key",
						Model:           tc.model,
						SmallFastModel:  tc.smallFastModel,
						BaseURL:         upstream.URL,
					}

					bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
						withCustomProvider(provider.NewAnthropic(anthropicCfg(upstream.URL, apiKey), bCfg)),
					)

					reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
					require.NoError(t, err)

					// Send with Anthropic-Beta header containing flags that should be filtered.
					resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody, http.Header{
						"Anthropic-Beta": {"interleaved-thinking-2025-05-14,effort-2025-11-24,context-management-2025-06-27,prompt-caching-scope-2026-01-05"},
					})
					require.NoError(t, err)
					defer resp.Body.Close()
					require.Equal(t, http.StatusOK, resp.StatusCode)
					_, err = io.ReadAll(resp.Body)
					require.NoError(t, err)

					received := upstream.receivedRequests()
					require.Len(t, received, 1)
					body := received[0].Body

					// Verify strippable fields: kept only if listed in expectKeptFields.
					for _, field := range strippableFields {
						assert.Equal(t, slices.Contains(tc.expectKeptFields, field), gjson.GetBytes(body, field).Exists(), "field %s", field)
					}

					// Verify thinking behavior.
					assert.Equal(t, tc.expectThinkingType, gjson.GetBytes(body, "thinking.type").String(), "thinking type mismatch")
					if tc.expectBudgetTokens > 0 {
						assert.Equal(t, tc.expectBudgetTokens, gjson.GetBytes(body, "thinking.budget_tokens").Int(), "budget_tokens mismatch")
					} else {
						assert.False(t, gjson.GetBytes(body, "thinking.budget_tokens").Exists(), "budget_tokens should not be present")
					}

					// The Bedrock SDK middleware moves Anthropic-Beta from the header
					// into the body as "anthropic_beta".
					betaArr := gjson.GetBytes(body, "anthropic_beta").Array()
					var gotFlags []string
					for _, v := range betaArr {
						gotFlags = append(gotFlags, v.String())
					}
					assert.Equal(t, tc.expectedBetaFlags, gotFlags, "beta flags mismatch")

					bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
				})
			}
		}
	})
}

func TestOpenAIChatCompletions(t *testing.T) {
	t.Parallel()

	t.Run("single builtin tool", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name                                      string
			streaming                                 bool
			expectedInputTokens, expectedOutputTokens int
			expectedToolCallID                        string
		}{
			{
				name:                 "streaming",
				streaming:            true,
				expectedInputTokens:  60,
				expectedOutputTokens: 15,
				expectedToolCallID:   "call_HjeqP7YeRkoNj0de9e3U4X4B",
			},
			{
				name:                 "non-streaming",
				streaming:            false,
				expectedInputTokens:  60,
				expectedOutputTokens: 15,
				expectedToolCallID:   "call_KjzAbhiZC6nk81tQzL7pwlpc",
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, fixtures.OaiChatSingleBuiltinTool)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

				// Make API call to aibridge for OpenAI /v1/chat/completions
				reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
				require.NoError(t, err)
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)

				// Response-specific checks.
				if tc.streaming {
					sp := aibridge.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))

					// OpenAI sends all events under the same type.
					messageEvents := sp.MessageEvents()
					assert.NotEmpty(t, messageEvents)

					// OpenAI streaming ends with [DONE]
					lastEvent := messageEvents[len(messageEvents)-1]
					assert.Equal(t, "[DONE]", lastEvent.Data)
				}

				tokenUsages := bridgeServer.Recorder.RecordedTokenUsages()
				require.Len(t, tokenUsages, 1)
				assert.EqualValues(t, tc.expectedInputTokens, bridgeServer.Recorder.TotalInputTokens(), "input tokens miscalculated")
				assert.EqualValues(t, tc.expectedOutputTokens, bridgeServer.Recorder.TotalOutputTokens(), "output tokens miscalculated")

				toolUsages := bridgeServer.Recorder.RecordedToolUsages()
				require.Len(t, toolUsages, 1)
				assert.Equal(t, "read_file", toolUsages[0].Tool)
				assert.Equal(t, tc.expectedToolCallID, toolUsages[0].ToolCallID)
				require.IsType(t, map[string]any{}, toolUsages[0].Args)
				require.Contains(t, toolUsages[0].Args, "path")
				assert.Equal(t, "README.md", toolUsages[0].Args.(map[string]any)["path"])

				promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
				require.Len(t, promptUsages, 1)
				assert.Equal(t, "how large is the README.md file in my current path", promptUsages[0].Prompt)

				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})

	t.Run("streaming injected tool call edge cases", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name         string
			fixture      []byte
			expectedArgs map[string]any
		}{
			{
				name:         "tool call no preamble",
				fixture:      fixtures.OaiChatStreamingInjectedToolNoPreamble,
				expectedArgs: map[string]any{"owner": "me"},
			},
			{
				name:         "tool call with non-zero index",
				fixture:      fixtures.OaiChatStreamingInjectedToolNonzeroIndex,
				expectedArgs: nil, // No arguments in this fixture
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				// Setup mock server for multi-turn interaction.
				// First request → tool call response, second → tool response.
				fix := fixtures.Parse(t, tc.fixture)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix), newFixtureToolResponse(fix))

				// Setup MCP proxies with the tool from the fixture
				mockMCP := setupMCPForTest(t, defaultTracer)

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
					withMCP(mockMCP),
				)

				// Add the stream param to the request.
				reqBody, err := sjson.SetBytes(fix.Request(), "stream", true)
				require.NoError(t, err)
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)

				// Verify SSE headers are sent correctly
				require.Equal(t, "text/event-stream", resp.Header.Get("Content-Type"))
				require.Equal(t, "no-cache", resp.Header.Get("Cache-Control"))
				require.Equal(t, "keep-alive", resp.Header.Get("Connection"))

				// Consume the full response body to ensure the interception completes
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				// Verify the MCP tool was actually invoked
				invocations := mockMCP.getCallsByTool(mockToolName)
				require.Len(t, invocations, 1, "expected MCP tool to be invoked")

				// Verify tool was invoked with the expected args (if specified)
				if tc.expectedArgs != nil {
					expected, err := json.Marshal(tc.expectedArgs)
					require.NoError(t, err)
					actual, err := json.Marshal(invocations[0])
					require.NoError(t, err)
					require.EqualValues(t, expected, actual)
				}

				// Verify tool usage was recorded
				toolUsages := bridgeServer.Recorder.RecordedToolUsages()
				require.Len(t, toolUsages, 1)
				assert.Equal(t, mockToolName, toolUsages[0].Tool)

				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})
}

func TestSimple(t *testing.T) {
	t.Parallel()

	getAnthropicResponseID := func(streaming bool, resp *http.Response) (string, error) {
		if streaming {
			decoder := ssestream.NewDecoder(resp)
			stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](decoder, nil)
			var message anthropic.Message
			for stream.Next() {
				event := stream.Current()
				if err := message.Accumulate(event); err != nil {
					return "", xerrors.Errorf("accumulate event: %w", err)
				}
			}
			if stream.Err() != nil {
				return "", xerrors.Errorf("stream error: %w", stream.Err())
			}
			return message.ID, nil
		}

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", xerrors.Errorf("read body: %w", err)
		}

		var message anthropic.Message
		if err := json.Unmarshal(body, &message); err != nil {
			return "", xerrors.Errorf("unmarshal response: %w", err)
		}
		return message.ID, nil
	}

	getOpenAIResponseID := func(streaming bool, resp *http.Response) (string, error) {
		if streaming {
			// Parse the response stream.
			decoder := oaissestream.NewDecoder(resp)
			stream := oaissestream.NewStream[openai.ChatCompletionChunk](decoder, nil)
			var message openai.ChatCompletionAccumulator
			for stream.Next() {
				chunk := stream.Current()
				message.AddChunk(chunk)
			}
			if stream.Err() != nil {
				return "", xerrors.Errorf("stream error: %w", stream.Err())
			}
			return message.ID, nil
		}

		// Parse & unmarshal the response.
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return "", xerrors.Errorf("read body: %w", err)
		}

		var message openai.ChatCompletion
		if err := json.Unmarshal(body, &message); err != nil {
			return "", xerrors.Errorf("unmarshal response: %w", err)
		}
		return message.ID, nil
	}

	testCases := []struct {
		name              string
		fixture           []byte
		basePath          string
		expectedPath      string
		getResponseIDFunc func(streaming bool, resp *http.Response) (string, error)
		path              string
		expectedMsgID     string
		userAgent         string
		expectedClient    aibridge.Client
	}{
		{
			name:              config.ProviderAnthropic,
			fixture:           fixtures.AntSimple,
			basePath:          "",
			expectedPath:      "/v1/messages",
			getResponseIDFunc: getAnthropicResponseID,
			path:              pathAnthropicMessages,
			expectedMsgID:     "msg_01Pvyf26bY17RcjmWfJsXGBn",
			userAgent:         "claude-cli/2.0.67 (external, cli)",
			expectedClient:    aibridge.ClientClaudeCode,
		},
		{
			name:              config.ProviderAnthropic + "_haiku_prompt_capture",
			fixture:           fixtures.AntHaikuSimple,
			basePath:          "",
			expectedPath:      "/v1/messages",
			getResponseIDFunc: getAnthropicResponseID,
			path:              pathAnthropicMessages,
			expectedMsgID:     "msg_01Pvyf26bY17RcjmWfJsXGBn",
			userAgent:         "claude-cli/2.0.67 (external, cli)",
			expectedClient:    aibridge.ClientClaudeCode,
		},
		{
			name:              config.ProviderOpenAI,
			fixture:           fixtures.OaiChatSimple,
			basePath:          "",
			expectedPath:      "/chat/completions",
			getResponseIDFunc: getOpenAIResponseID,
			path:              pathOpenAIChatCompletions,
			expectedMsgID:     "chatcmpl-BwoiPTGRbKkY5rncfaM0s9KtWrq5N",
			userAgent:         "codex_cli_rs/0.87.0 (Mac OS 26.2.0; arm64)",
			expectedClient:    aibridge.ClientCodex,
		},
		{
			name:              config.ProviderAnthropic + "_baseURL_path",
			fixture:           fixtures.AntSimple,
			basePath:          "/api",
			expectedPath:      "/api/v1/messages",
			getResponseIDFunc: getAnthropicResponseID,
			path:              pathAnthropicMessages,
			expectedMsgID:     "msg_01Pvyf26bY17RcjmWfJsXGBn",
			userAgent:         "GitHubCopilotChat/0.37.2026011603",
			expectedClient:    aibridge.ClientCopilotVSC,
		},
		{
			name:              config.ProviderOpenAI + "_baseURL_path",
			fixture:           fixtures.OaiChatSimple,
			basePath:          "/api",
			expectedPath:      "/api/chat/completions",
			getResponseIDFunc: getOpenAIResponseID,
			path:              pathOpenAIChatCompletions,
			expectedMsgID:     "chatcmpl-BwoiPTGRbKkY5rncfaM0s9KtWrq5N",
			userAgent:         "Zed/0.219.4+stable.119.abc123 (macos; aarch64)",
			expectedClient:    aibridge.ClientZed,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			for _, streaming := range []bool{true, false} {
				t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
					t.Parallel()

					ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
					t.Cleanup(cancel)

					fix := fixtures.Parse(t, tc.fixture)
					upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

					bridgeServer := newBridgeTestServer(ctx, t, upstream.URL+tc.basePath)

					// When: calling the "API server" with the fixture's request body.
					reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
					require.NoError(t, err)
					resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody, http.Header{"User-Agent": {tc.userAgent}})
					require.NoError(t, err)
					defer resp.Body.Close()
					require.Equal(t, http.StatusOK, resp.StatusCode)

					// Then: I expect the upstream request to have the correct path.
					received := upstream.receivedRequests()
					require.Len(t, received, 1)
					require.Equal(t, tc.expectedPath, received[0].Path)

					// Then: I expect a non-empty response.
					bodyBytes, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					assert.NotEmpty(t, bodyBytes, "should have received response body")

					// Reset the body after being read.
					resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

					// Then: I expect the prompt to have been tracked.
					promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
					require.NotEmpty(t, promptUsages, "no prompts tracked")
					assert.Contains(t, promptUsages[0].Prompt, "how many angels can dance on the head of a pin")

					// Validate that responses have their IDs overridden with a interception ID rather than the original ID from the upstream provider.
					// The reason for this is that Bridge may make multiple upstream requests (i.e. to invoke injected tools), and clients will not be expecting
					// multiple messages in response to a single request.
					id, err := tc.getResponseIDFunc(streaming, resp)
					require.NoError(t, err, "failed to retrieve response ID")
					require.Nilf(t, uuid.Validate(id), "%s is not a valid UUID", id)

					tokenUsages := bridgeServer.Recorder.RecordedTokenUsages()
					require.GreaterOrEqual(t, len(tokenUsages), 1)
					require.Equal(t, tokenUsages[0].MsgID, tc.expectedMsgID)

					// Validate user agent and client have been recorded.
					interceptions := bridgeServer.Recorder.RecordedInterceptions()
					require.Len(t, interceptions, 1, "expected exactly one interception, got: %v", interceptions)
					assert.Equal(t, id, interceptions[0].ID)
					assert.Equal(t, tc.userAgent, interceptions[0].UserAgent)
					assert.Equal(t, string(tc.expectedClient), interceptions[0].Client)

					bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
				})
			}
		})
	}
}

func TestSessionIDTracking(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name              string
		fixture           []byte
		header            http.Header
		metadataSessionID string
		expectedClient    aibridge.Client
		expectSessionID   string
	}{
		// Session in header.
		{
			name:            "mux",
			fixture:         fixtures.AntSimple,
			expectedClient:  aibridge.ClientMux,
			expectSessionID: "mux-workspace-321",
			header: http.Header{
				"User-Agent":         []string{"mux/1.0.0"},
				"X-Mux-Workspace-Id": []string{"mux-workspace-321"},
			},
		},
		// Session in body.
		{
			name:            "claude_code",
			fixture:         fixtures.AntSimple,
			expectedClient:  aibridge.ClientClaudeCode,
			expectSessionID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
			header: http.Header{
				"User-Agent": []string{"claude-cli/2.0.67 (external, cli)"},
			},
			metadataSessionID: "user_abc123_account_456_session_f47ac10b-58cc-4372-a567-0e02b2c3d479",
		},
		// No session.
		{
			name:           "zed",
			fixture:        fixtures.AntSimple,
			expectedClient: aibridge.ClientZed,
			header: http.Header{
				"User-Agent": []string{"Zed/0.219.4+stable.119.abc123 (macos; aarch64)"},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL, withProvider(config.ProviderAnthropic))

			reqBody := fix.Request()
			if tc.metadataSessionID != "" {
				var err error
				reqBody, err = sjson.SetBytes(reqBody, "metadata.user_id", tc.metadataSessionID)
				require.NoError(t, err)
			}

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody, tc.header)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Drain the body to let the stream complete.
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			interceptions := bridgeServer.Recorder.RecordedInterceptions()
			require.Len(t, interceptions, 1, "expected exactly one interception")
			assert.Equal(t, string(tc.expectedClient), interceptions[0].Client)

			if tc.expectSessionID == "" {
				assert.Nil(t, interceptions[0].ClientSessionID, "expected nil session ID for %s", tc.name)
			} else {
				require.NotNil(t, interceptions[0].ClientSessionID, "expected non-nil session ID for %s", tc.name)
				assert.Equal(t, tc.expectSessionID, *interceptions[0].ClientSessionID)
			}

			bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
		})
	}
}

func TestFallthrough(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name                 string
		fixture              []byte
		basePath             string
		requestPath          string
		expectedUpstreamPath string
		expectAuthHeader     string
	}{
		{
			name:                 "ant_empty_base_url_path",
			fixture:              fixtures.AntFallthrough,
			basePath:             "",
			requestPath:          "/anthropic/v1/models",
			expectedUpstreamPath: "/v1/models",
			expectAuthHeader:     "X-Api-Key",
		},
		{
			name:                 "oai_empty_base_url_path",
			fixture:              fixtures.OaiChatFallthrough,
			basePath:             "",
			requestPath:          "/openai/v1/models",
			expectedUpstreamPath: "/models",
			expectAuthHeader:     "Authorization",
		},
		{
			name:                 "ant_some_base_url_path",
			fixture:              fixtures.AntFallthrough,
			basePath:             "/api",
			requestPath:          "/anthropic/v1/models",
			expectedUpstreamPath: "/api/v1/models",
			expectAuthHeader:     "X-Api-Key",
		},
		{
			name:                 "oai_some_base_url_path",
			fixture:              fixtures.OaiChatFallthrough,
			basePath:             "/api",
			requestPath:          "/openai/v1/models",
			expectedUpstreamPath: "/api/models",
			expectAuthHeader:     "Authorization",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(t.Context(), t, newFixtureResponse(fix))
			bridgeServer := newBridgeTestServer(t.Context(), t, upstream.URL+tc.basePath)

			resp, err := bridgeServer.makeRequest(t, http.MethodGet, tc.requestPath, nil)
			require.NoError(t, err)
			defer resp.Body.Close()

			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify upstream received the request at the expected path
			// with the API key header.
			received := upstream.receivedRequests()
			require.Len(t, received, 1)
			require.Equal(t, tc.expectedUpstreamPath, received[0].Path)
			require.Contains(t, received[0].Header.Get(tc.expectAuthHeader), apiKey)

			gotBytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Compare JSON bodies for semantic equality.
			var got any
			var exp any
			require.NoError(t, json.Unmarshal(gotBytes, &got))
			require.NoError(t, json.Unmarshal(fix.NonStreaming(), &exp))
			require.EqualValues(t, exp, got)
		})
	}
}

func TestAnthropicInjectedTools(t *testing.T) {
	t.Parallel()

	for _, streaming := range []bool{true, false} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			t.Parallel()

			// Build the requirements & make the assertions which are common to all providers.
			bridgeServer, mockMCP, resp := setupInjectedToolTest(t, fixtures.AntSingleInjectedTool, streaming, defaultTracer, pathAnthropicMessages, anthropicToolResultValidator(t))
			defer resp.Body.Close()

			// Ensure expected tool was invoked with expected input.
			toolUsages := bridgeServer.Recorder.RecordedToolUsages()
			require.Len(t, toolUsages, 1)
			require.Equal(t, mockToolName, toolUsages[0].Tool)
			expected, err := json.Marshal(map[string]any{"owner": "admin"})
			require.NoError(t, err)
			actual, err := json.Marshal(toolUsages[0].Args)
			require.NoError(t, err)
			require.EqualValues(t, expected, actual)
			invocations := mockMCP.getCallsByTool(mockToolName)
			require.Len(t, invocations, 1)
			actual, err = json.Marshal(invocations[0])
			require.NoError(t, err)
			require.EqualValues(t, expected, actual)

			var (
				content *anthropic.ContentBlockUnion
				message anthropic.Message
			)
			if streaming {
				// Parse the response stream.
				decoder := ssestream.NewDecoder(resp)
				stream := ssestream.NewStream[anthropic.MessageStreamEventUnion](decoder, nil)
				for stream.Next() {
					event := stream.Current()
					require.NoError(t, message.Accumulate(event), "accumulate event")
				}

				require.NoError(t, stream.Err(), "stream error")
				require.Len(t, message.Content, 2)

				content = &message.Content[1]
			} else {
				// Parse & unmarshal the response.
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "read response body")

				require.NoError(t, json.Unmarshal(body, &message), "unmarshal response")
				require.GreaterOrEqual(t, len(message.Content), 1)

				content = &message.Content[0]
			}

			// Ensure tool returned expected value.
			require.NotNil(t, content)
			require.Contains(t, content.Text, "dd711d5c-83c6-4c08-a0af-b73055906e8c") // The ID of the workspace to be returned.

			// Check the token usage from the client's perspective.
			//
			// We overwrite the final message_delta which is relayed to the client to include the
			// accumulated tokens but currently the SDK only supports accumulating output tokens
			// for message_delta events.
			//
			// For non-streaming requests the token usage is also overwritten and should be faithfully
			// represented in the response.
			//
			// See https://github.com/anthropics/anthropic-sdk-go/blob/v1.12.0/message.go#L2619-L2622
			if !streaming {
				assert.EqualValues(t, 15308, message.Usage.InputTokens)
			}
			assert.EqualValues(t, 204, message.Usage.OutputTokens)

			// Ensure tokens used during injected tool invocation are accounted for.
			assert.EqualValues(t, 15308, bridgeServer.Recorder.TotalInputTokens())
			assert.EqualValues(t, 204, bridgeServer.Recorder.TotalOutputTokens())

			// Ensure we received exactly one prompt.
			promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
			require.Len(t, promptUsages, 1)
		})
	}
}

func TestOpenAIInjectedTools(t *testing.T) {
	t.Parallel()

	for _, streaming := range []bool{true, false} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			t.Parallel()

			// Build the requirements & make the assertions which are common to all providers.
			bridgeServer, mockMCP, resp := setupInjectedToolTest(t, fixtures.OaiChatSingleInjectedTool, streaming, defaultTracer, pathOpenAIChatCompletions, openaiChatToolResultValidator(t))
			defer resp.Body.Close()

			// Ensure expected tool was invoked with expected input.
			toolUsages := bridgeServer.Recorder.RecordedToolUsages()
			require.Len(t, toolUsages, 1)
			require.Equal(t, mockToolName, toolUsages[0].Tool)
			expected, err := json.Marshal(map[string]any{"owner": "admin"})
			require.NoError(t, err)
			actual, err := json.Marshal(toolUsages[0].Args)
			require.NoError(t, err)
			require.EqualValues(t, expected, actual)
			invocations := mockMCP.getCallsByTool(mockToolName)
			require.Len(t, invocations, 1)
			actual, err = json.Marshal(invocations[0])
			require.NoError(t, err)
			require.EqualValues(t, expected, actual)

			var (
				content *openai.ChatCompletionChoice
				message openai.ChatCompletion
			)
			if streaming {
				// Parse the response stream.
				decoder := oaissestream.NewDecoder(resp)
				stream := oaissestream.NewStream[openai.ChatCompletionChunk](decoder, nil)
				var acc openai.ChatCompletionAccumulator
				detectedToolCalls := make(map[string]struct{})
				for stream.Next() {
					chunk := stream.Current()
					acc.AddChunk(chunk)

					if len(chunk.Choices) == 0 {
						continue
					}

					for _, c := range chunk.Choices {
						if len(c.Delta.ToolCalls) == 0 {
							continue
						}

						for _, t := range c.Delta.ToolCalls {
							if t.Function.Name == "" {
								continue
							}

							detectedToolCalls[t.Function.Name] = struct{}{}
						}
					}
				}

				// Verify that no injected tool call events (or partials thereof) were sent to the client.
				require.Len(t, detectedToolCalls, 0)

				message = acc.ChatCompletion
				require.NoError(t, stream.Err(), "stream error")
			} else {
				// Parse & unmarshal the response.
				body, err := io.ReadAll(resp.Body)
				require.NoError(t, err, "read response body")
				require.NoError(t, json.Unmarshal(body, &message), "unmarshal response")

				// Verify that no injected tools were sent to the client.
				require.GreaterOrEqual(t, len(message.Choices), 1)
				require.Len(t, message.Choices[0].Message.ToolCalls, 0)
			}

			require.GreaterOrEqual(t, len(message.Choices), 1)
			content = &message.Choices[0]

			// Ensure tool returned expected value.
			require.NotNil(t, content)
			require.Contains(t, content.Message.Content, "dd711d5c-83c6-4c08-a0af-b73055906e8c") // The ID of the workspace to be returned.

			// Check the token usage from the client's perspective.
			// This *should* work but the openai SDK doesn't accumulate the prompt token details :(.
			// See https://github.com/openai/openai-go/blob/v2.7.0/streamaccumulator.go#L145-L147.
			// assert.EqualValues(t, 5047, message.Usage.PromptTokens-message.Usage.PromptTokensDetails.CachedTokens)
			assert.EqualValues(t, 105, message.Usage.CompletionTokens)

			// Ensure tokens used during injected tool invocation are accounted for.
			require.EqualValues(t, 5047, bridgeServer.Recorder.TotalInputTokens())
			require.EqualValues(t, 105, bridgeServer.Recorder.TotalOutputTokens())

			// Ensure we received exactly one prompt.
			promptUsages := bridgeServer.Recorder.RecordedPromptUsages()
			require.Len(t, promptUsages, 1)
		})
	}
}

// anthropicToolResultValidator returns a request validator that asserts the second
// upstream request contains the assistant's tool_use and user's tool_result messages
// appended by the inner agentic loop. If the raw payload is not kept in sync with
// the structured messages, the second request will be identical to the first.
func anthropicToolResultValidator(t *testing.T) func(*http.Request, []byte) {
	t.Helper()

	return func(_ *http.Request, raw []byte) {
		messages := gjson.GetBytes(raw, "messages").Array()

		// After the agentic loop the messages must contain at minimum:
		//   [0]   original user message
		//   [N-2] assistant message with tool_use content block
		//   [N-1] user message with tool_result content block
		require.GreaterOrEqual(t, len(messages), 3,
			"second upstream request must contain the original message, assistant tool_use, and user tool_result")

		assistantMsg := messages[len(messages)-2]
		require.Equal(t, "assistant", assistantMsg.Get("role").Str,
			"penultimate message must be from the assistant")
		var hasToolUse bool
		for _, block := range assistantMsg.Get("content").Array() {
			if block.Get("type").Str == "tool_use" {
				hasToolUse = true
				break
			}
		}
		require.True(t, hasToolUse, "assistant message must contain a tool_use content block")

		toolResultMsg := messages[len(messages)-1]
		require.Equal(t, "user", toolResultMsg.Get("role").Str,
			"last message must be a user message carrying the tool_result")
		var hasToolResult bool
		for _, block := range toolResultMsg.Get("content").Array() {
			if block.Get("type").Str == "tool_result" {
				hasToolResult = true
				break
			}
		}
		require.True(t, hasToolResult, "user message must contain a tool_result content block")
	}
}

// openaiChatToolResultValidator returns a request validator that asserts the second
// upstream request contains the assistant's tool_calls and a role=tool result message
// appended by the inner agentic loop.
func openaiChatToolResultValidator(t *testing.T) func(*http.Request, []byte) {
	t.Helper()

	return func(_ *http.Request, raw []byte) {
		messages := gjson.GetBytes(raw, "messages").Array()

		// After the agentic loop the messages must contain at minimum:
		//   [0]   original user message
		//   [N-2] assistant message with tool_calls array
		//   [N-1] message with role=tool
		require.GreaterOrEqual(t, len(messages), 3,
			"second upstream request must contain the original message, assistant tool_calls, and tool result")

		assistantMsg := messages[len(messages)-2]
		require.Equal(t, "assistant", assistantMsg.Get("role").Str,
			"penultimate message must be from the assistant")
		require.NotEmpty(t, len(assistantMsg.Get("tool_calls").Array()),
			"assistant message must contain a tool_calls array")

		toolResultMsg := messages[len(messages)-1]
		require.Equal(t, "tool", toolResultMsg.Get("role").Str,
			"last message must have role=tool")
		require.NotEmpty(t, toolResultMsg.Get("tool_call_id").Str,
			"tool result message must have a tool_call_id")
	}
}

func TestErrorHandling(t *testing.T) {
	t.Parallel()

	// Tests that errors which occur *before* a streaming response begins, or in non-streaming requests, are handled as expected.
	t.Run("non-stream error", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name              string
			fixture           []byte
			path              string
			responseHandlerFn func(resp *http.Response)
		}{
			{
				name:    config.ProviderAnthropic,
				fixture: fixtures.AntNonStreamError,
				path:    pathAnthropicMessages,
				responseHandlerFn: func(resp *http.Response) {
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Equal(t, "error", gjson.GetBytes(body, "type").Str)
					require.Equal(t, "invalid_request_error", gjson.GetBytes(body, "error.type").Str)
					require.Contains(t, gjson.GetBytes(body, "error.message").Str, "prompt is too long")
				},
			},
			{
				name:    config.ProviderOpenAI,
				fixture: fixtures.OaiChatNonStreamError,
				path:    pathOpenAIChatCompletions,
				responseHandlerFn: func(resp *http.Response) {
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					body, err := io.ReadAll(resp.Body)
					require.NoError(t, err)
					require.Equal(t, "context_length_exceeded", gjson.GetBytes(body, "error.code").Str)
					require.Equal(t, "invalid_request_error", gjson.GetBytes(body, "error.type").Str)
					require.Contains(t, gjson.GetBytes(body, "error.message").Str, "Input tokens exceed the configured limit")
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				for _, streaming := range []bool{true, false} {
					t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
						t.Parallel()

						ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
						t.Cleanup(cancel)

						// Setup mock server. Error fixtures contain raw HTTP
						// responses that may cause the bridge to retry.
						fix := fixtures.Parse(t, tc.fixture)
						upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

						bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

						// Add the stream param to the request.
						reqBody, err := sjson.SetBytes(fix.Request(), "stream", streaming)
						require.NoError(t, err)

						resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody)
						require.NoError(t, err)
						defer resp.Body.Close()

						tc.responseHandlerFn(resp)
						bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
					})
				}
			})
		}
	})

	// Tests that errors which occur *during* a streaming response are handled as expected.
	t.Run("mid-stream error", func(t *testing.T) {
		cases := []struct {
			name              string
			fixture           []byte
			path              string
			responseHandlerFn func(resp *http.Response)
		}{
			{
				name:    config.ProviderAnthropic,
				fixture: fixtures.AntMidStreamError,
				path:    pathAnthropicMessages,
				responseHandlerFn: func(resp *http.Response) {
					// Server responds first with 200 OK then starts streaming.
					require.Equal(t, http.StatusOK, resp.StatusCode)

					sp := aibridge.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))
					require.Len(t, sp.EventsByType("error"), 1)
					require.Contains(t, sp.EventsByType("error")[0].Data, "Overloaded")
				},
			},
			{
				name:    config.ProviderOpenAI,
				fixture: fixtures.OaiChatMidStreamError,
				path:    pathOpenAIChatCompletions,
				responseHandlerFn: func(resp *http.Response) {
					// Server responds first with 200 OK then starts streaming.
					require.Equal(t, http.StatusOK, resp.StatusCode)

					sp := aibridge.NewSSEParser()
					require.NoError(t, sp.Parse(resp.Body))
					// OpenAI sends all events under the same type.
					messageEvents := sp.MessageEvents()
					require.NotEmpty(t, messageEvents)

					errEvent := sp.MessageEvents()[len(sp.MessageEvents())-2] // Last event is termination marker ("[DONE]").
					require.NotEmpty(t, errEvent)
					require.Contains(t, errEvent.Data, "The server had an error while processing your request. Sorry about that!")
				},
			},
		}

		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				// Setup mock server.
				fix := fixtures.Parse(t, tc.fixture)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))
				upstream.StatusCode = http.StatusInternalServerError

				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, fix.Request())
				require.NoError(t, err)
				defer resp.Body.Close()

				tc.responseHandlerFn(resp)
				bridgeServer.Recorder.VerifyAllInterceptionsEnded(t)
			})
		}
	})
}

// TestStableRequestEncoding validates that a given intercepted request and a
// given set of injected tools should result identical payloads.
//
// Should the payload vary, it may subvert any caching mechanisms the provider may have.
func TestStableRequestEncoding(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		fixture []byte
		path    string
	}{
		{
			name:    config.ProviderAnthropic,
			fixture: fixtures.AntSimple,
			path:    pathAnthropicMessages,
		},
		{
			name:    config.ProviderOpenAI,
			fixture: fixtures.OaiChatSimple,
			path:    pathOpenAIChatCompletions,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Setup MCP tools.
			mockMCP := setupMCPForTest(t, defaultTracer)

			fix := fixtures.Parse(t, tc.fixture)

			// Create a mock upstream that serves the same blocking response for each request.
			count := 10
			responses := make([]upstreamResponse, count)
			for i := range count {
				responses[i] = newFixtureResponse(fix)
			}
			upstream := newMockUpstream(ctx, t, responses...)

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withMCP(mockMCP),
			)

			// Make multiple requests and verify they all have identical payloads.
			for range count {
				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, fix.Request())
				require.NoError(t, err)
				defer resp.Body.Close()
				require.Equal(t, http.StatusOK, resp.StatusCode)
			}

			// All upstream request bodies should be identical.
			received := upstream.receivedRequests()
			require.Len(t, received, count)
			reference := string(received[0].Body)
			for _, r := range received[1:] {
				assert.JSONEq(t, reference, string(r.Body))
			}
		})
	}
}

// TestAnthropicToolChoiceParallelDisabled verifies that parallel tool use is
// correctly disabled based on the tool_choice parameter in the request.
// See https://github.com/coder/aibridge/issues/2
func TestAnthropicToolChoiceParallelDisabled(t *testing.T) {
	t.Parallel()

	var (
		toolChoiceAuto = string(constant.ValueOf[constant.Auto]())
		toolChoiceAny  = string(constant.ValueOf[constant.Any]())
		toolChoiceNone = string(constant.ValueOf[constant.None]())
		toolChoiceTool = string(constant.ValueOf[constant.Tool]())
	)

	cases := []struct {
		name                          string
		fixture                       []byte
		toolChoice                    any // nil, or map with "type" key.
		withInjectedTools             bool
		expectDisableParallel         *bool // nil = field should not be present, non-nil = expected value.
		expectToolChoiceTypeInRequest string
	}{
		// With injected tools - disable_parallel_tool_use should be set to true.
		{
			name:                          "with injected tools: no tool_choice defined defaults to auto",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    nil,
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected tools: tool_choice auto",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected tools: tool_choice any",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAny},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAny,
		},
		{
			name:                          "with injected tools: tool_choice tool",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceTool, "name": "some_tool"},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceTool,
		},
		{
			name:                          "with injected tools: tool_choice none",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceNone},
			withInjectedTools:             true,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceNone,
		},
		// With injected tools and builtin tools - disable_parallel_tool_use should be set to true.
		{
			name:                          "with injected and builtin tools: no tool_choice defined defaults to auto",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    nil,
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected and builtin tools: tool_choice auto",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected and builtin tools: tool_choice any",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAny},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAny,
		},
		{
			name:                          "with injected and builtin tools: tool_choice tool",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceTool, "name": "some_tool"},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceTool,
		},
		{
			name:                          "with injected and builtin tools: tool_choice none",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceNone},
			withInjectedTools:             true,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceNone,
		},
		{
			name:                          "with injected and builtin tools: request already disables parallel",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": true},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected and builtin tools: request explicitly enables parallel",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": false},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		// Without injected or builtin tools - disable_parallel_tool_use should NOT be set.
		{
			name:                          "without injected tools or builtin tools: tool_choice auto",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto},
			withInjectedTools:             false,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "without injected tools or builtin tools: tool_choice any",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAny},
			withInjectedTools:             false,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceAny,
		},
		// With builtin tools but without injected tools - disable_parallel_tool_use should NOT be set.
		{
			name:                          "with builtin tools only: tool_choice auto",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto},
			withInjectedTools:             false,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with builtin tools only: tool_choice any",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAny},
			withInjectedTools:             false,
			expectDisableParallel:         nil,
			expectToolChoiceTypeInRequest: toolChoiceAny,
		},
		{
			name:                          "with builtin tools only: request explicitly disables parallel",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": true},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with builtin tools only: request explicitly enables parallel",
			fixture:                       fixtures.AntSingleBuiltinTool,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": false},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(false),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		// Without injected or builtin tools - disable_parallel_tool_use should be preserved if set.
		{
			name:                          "no tools: request explicitly disables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": true},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "no tools: request explicitly enables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": false},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(false),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		// Request already has disable_parallel_tool_use set - with injected tools it should be set to true.
		{
			name:                          "with injected tools: request already disables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": true},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "with injected tools: request explicitly enables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": false},
			withInjectedTools:             true,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		// Request already has disable_parallel_tool_use set - without injected tools it should be preserved.
		{
			name:                          "without injected tools: request already disables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": true},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(true),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
		{
			name:                          "without injected tools: request explicitly enables parallel",
			fixture:                       fixtures.AntSimple,
			toolChoice:                    map[string]any{"type": toolChoiceAuto, "disable_parallel_tool_use": false},
			withInjectedTools:             false,
			expectDisableParallel:         utils.PtrTo(false),
			expectToolChoiceTypeInRequest: toolChoiceAuto,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Setup MCP tools conditionally.
			var mockMCP mcp.ServerProxier
			if tc.withInjectedTools {
				mockMCP = setupMCPForTest(t, defaultTracer)
			} else {
				mockMCP = newNoopMCPManager()
			}

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
				withMCP(mockMCP),
			)

			// Prepare request body with tool_choice set.
			reqBody, err := sjson.SetBytes(fix.Request(), "tool_choice", tc.toolChoice)
			require.NoError(t, err)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify tool_choice in the upstream request.
			received := upstream.receivedRequests()
			require.Len(t, received, 1)
			var receivedRequest map[string]any
			require.NoError(t, json.Unmarshal(received[0].Body, &receivedRequest))
			toolChoice, ok := receivedRequest["tool_choice"].(map[string]any)
			require.True(t, ok, "expected tool_choice in upstream request")

			// Verify the type matches expectation.
			assert.Equal(t, tc.expectToolChoiceTypeInRequest, toolChoice["type"])

			// Verify name is preserved for tool_choice=tool.
			if tc.expectToolChoiceTypeInRequest == toolChoiceTool {
				assert.Equal(t, "some_tool", toolChoice["name"])
			}

			// Verify disable_parallel_tool_use based on expectations.
			// See https://platform.claude.com/docs/en/agents-and-tools/tool-use/implement-tool-use#parallel-tool-use
			disableParallel, hasDisableParallel := toolChoice["disable_parallel_tool_use"].(bool)

			require.Equal(t, tc.expectDisableParallel != nil, hasDisableParallel,
				"disable_parallel_tool_use presence mismatch")
			if tc.expectDisableParallel != nil {
				assert.Equal(t, *tc.expectDisableParallel, disableParallel)
			}
		})
	}
}

// TestChatCompletionsParallelToolCallsDisabled verifies that parallel_tool_calls
// is set to false only when injectable MCP tools are present and the request
// includes tools.
func TestChatCompletionsParallelToolCallsDisabled(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name              string
		fixture           []byte
		withInjectedTools bool
		initialSetting    *bool
		expectedSetting   *bool
	}{
		// With injected tools and builtin tools: parallel_tool_calls should be forced false.
		{
			name:              "with injected and builtin tools: parallel_tool_calls true",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected and builtin tools: parallel_tool_calls false",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected and builtin tools: parallel_tool_calls unset",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: true,
			initialSetting:    nil,
			expectedSetting:   utils.PtrTo(false),
		},
		// With injected tools but without builtin tools: parallel_tool_calls should be forced false.
		{
			name:              "with injected tools only: parallel_tool_calls true",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected tools only: parallel_tool_calls false",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: true,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with injected tools only: parallel_tool_calls unset",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: true,
			initialSetting:    nil,
			expectedSetting:   utils.PtrTo(false),
		},
		// With builtin tools but without injected tools: parallel_tool_calls should be preserved.
		{
			name:              "with builtin tools only: parallel_tool_calls true",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(true),
		},
		{
			name:              "with builtin tools only: parallel_tool_calls false",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "with builtin tools only: parallel_tool_calls unset",
			fixture:           fixtures.OaiChatSingleBuiltinTool,
			withInjectedTools: false,
			initialSetting:    nil,
			expectedSetting:   nil,
		},
		// Without any tools: nothing is modified.
		{
			name:              "no tools: parallel_tool_calls true",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(true),
			expectedSetting:   utils.PtrTo(true),
		},
		{
			name:              "no tools: parallel_tool_calls false",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: false,
			initialSetting:    utils.PtrTo(false),
			expectedSetting:   utils.PtrTo(false),
		},
		{
			name:              "no tools: parallel_tool_calls unset",
			fixture:           fixtures.OaiChatSimple,
			withInjectedTools: false,
			initialSetting:    nil,
			expectedSetting:   nil,
		},
	}

	for _, tc := range cases {
		for _, streaming := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/streaming=%v", tc.name, streaming), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, tc.fixture)
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
				reqBody, err = sjson.SetBytes(reqBody, "stream", streaming)
				require.NoError(t, err)

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathOpenAIChatCompletions, reqBody)
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

func TestThinkingAdaptiveIsPreserved(t *testing.T) {
	t.Parallel()

	fix := fixtures.Parse(t, fixtures.AntSimple)

	for _, streaming := range []bool{true, false} {
		t.Run(fmt.Sprintf("streaming=%v", streaming), func(t *testing.T) {
			t.Parallel()

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			// Create a mock server that captures the request body sent upstream.
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			// Inject adaptive thinking into the fixture request.
			reqBody, err := sjson.SetBytes(fix.Request(), "thinking", map[string]string{"type": "adaptive"})
			require.NoError(t, err)
			reqBody, err = sjson.SetBytes(reqBody, "stream", streaming)
			require.NoError(t, err)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, pathAnthropicMessages, reqBody)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)
			_, err = io.ReadAll(resp.Body)
			require.NoError(t, err)

			// Verify the thinking field was preserved in the upstream request.
			received := upstream.receivedRequests()
			require.Len(t, received, 1)
			assert.Equal(t, "adaptive", gjson.GetBytes(received[0].Body, "thinking.type").Str)
		})
	}
}

func TestEnvironmentDoNotLeak(t *testing.T) {
	// NOTE: Cannot use t.Parallel() here because subtests use t.Setenv which requires sequential execution.

	// Test that environment variables containing API keys/tokens are not leaked to upstream requests.
	// See https://github.com/coder/aibridge/issues/60.
	testCases := []struct {
		name       string
		fixture    []byte
		path       string
		envVars    map[string]string
		headerName string
	}{
		{
			name:    config.ProviderAnthropic,
			fixture: fixtures.AntSimple,
			path:    pathAnthropicMessages,
			envVars: map[string]string{
				"ANTHROPIC_AUTH_TOKEN": "should-not-leak",
			},
			headerName: "Authorization", // We only send through the X-Api-Key, so this one should not be present.
		},
		{
			name:    config.ProviderOpenAI,
			fixture: fixtures.OaiChatSimple,
			path:    pathOpenAIChatCompletions,
			envVars: map[string]string{
				"OPENAI_ORG_ID": "should-not-leak",
			},
			headerName: "OpenAI-Organization",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// NOTE: Cannot use t.Parallel() here because t.Setenv requires sequential execution.

			ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
			t.Cleanup(cancel)

			fix := fixtures.Parse(t, tc.fixture)
			upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

			// Set environment variables that the SDK would automatically read.
			// These should NOT leak into upstream requests.
			for key, val := range tc.envVars {
				t.Setenv(key, val)
			}

			bridgeServer := newBridgeTestServer(ctx, t, upstream.URL)

			resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, fix.Request())
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, http.StatusOK, resp.StatusCode)

			// Verify that environment values did not leak.
			received := upstream.receivedRequests()
			require.Len(t, received, 1)
			require.Empty(t, received[0].Header.Get(tc.headerName))
		})
	}
}

func TestActorHeaders(t *testing.T) {
	t.Parallel()

	actorUsername := "bob"

	cases := []struct {
		name             string
		path             string
		createProviderFn func(url, key string, sendHeaders bool) aibridge.Provider
		fixture          []byte
		streaming        bool
	}{
		{
			name: "openai/v1/chat/completions",
			path: pathOpenAIChatCompletions,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := openAICfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewOpenAI(cfg)
			},
			fixture:   fixtures.OaiChatSimple,
			streaming: true,
		},
		{
			name: "openai/v1/chat/completions",
			path: pathOpenAIChatCompletions,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := openAICfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewOpenAI(cfg)
			},
			fixture:   fixtures.OaiChatSimple,
			streaming: false,
		},
		{
			name: "openai/v1/responses",
			path: pathOpenAIResponses,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := openAICfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewOpenAI(cfg)
			},
			fixture:   fixtures.OaiResponsesStreamingSimple,
			streaming: true,
		},
		{
			name: "openai/v1/responses",
			path: pathOpenAIResponses,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := openAICfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewOpenAI(cfg)
			},
			fixture:   fixtures.OaiResponsesBlockingSimple,
			streaming: false,
		},
		{
			name: "anthropic/v1/messages",
			path: pathAnthropicMessages,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := anthropicCfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewAnthropic(cfg, nil)
			},
			fixture:   fixtures.AntSimple,
			streaming: true,
		},
		{
			name: "anthropic/v1/messages",
			path: pathAnthropicMessages,
			createProviderFn: func(url, key string, sendHeaders bool) aibridge.Provider {
				cfg := anthropicCfg(url, key)
				cfg.SendActorHeaders = sendHeaders
				return provider.NewAnthropic(cfg, nil)
			},
			fixture:   fixtures.AntSimple,
			streaming: false,
		},
	}

	for _, tc := range cases {
		for _, send := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/streaming=%v/send-headers=%v", tc.name, tc.streaming, send), func(t *testing.T) {
				t.Parallel()

				ctx, cancel := context.WithTimeout(t.Context(), testutil.WaitLong)
				t.Cleanup(cancel)

				fix := fixtures.Parse(t, tc.fixture)
				upstream := newMockUpstream(ctx, t, newFixtureResponse(fix))

				metadataKey := "Username"
				bridgeServer := newBridgeTestServer(ctx, t, upstream.URL,
					withCustomProvider(tc.createProviderFn(upstream.URL, apiKey, send)),
					withActor(defaultActorID, recorder.Metadata{
						metadataKey: actorUsername,
					}),
				)

				// Add the stream param to the request.
				reqBody, err := sjson.SetBytes(fix.Request(), "stream", tc.streaming)
				require.NoError(t, err)

				resp, err := bridgeServer.makeRequest(t, http.MethodPost, tc.path, reqBody)
				require.NoError(t, err)
				defer resp.Body.Close()
				// Drain the body so streaming responses complete without
				// a "connection reset" error in the mock upstream.
				_, err = io.ReadAll(resp.Body)
				require.NoError(t, err)

				received := upstream.receivedRequests()
				require.NotEmpty(t, received)
				receivedHeaders := received[0].Header

				// Verify that the actor headers were only received if intended.
				found := make(map[string][]string)
				for k, v := range receivedHeaders {
					k = strings.ToLower(k)
					if intercept.IsActorHeader(k) {
						found[k] = v
					}
				}

				if send {
					require.Equal(t, found[strings.ToLower(intercept.ActorIDHeader())], []string{defaultActorID})
					require.Equal(t, found[strings.ToLower(intercept.ActorMetadataHeader(metadataKey))], []string{actorUsername})
				} else {
					require.Empty(t, found)
				}
			})
		}
	}
}
