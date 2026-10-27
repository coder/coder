package chattest_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatd/chattest"
	"github.com/coder/coder/v2/testutil"
)

var openAIAPIs = []struct {
	name string
	opts []fantasyopenai.Option
}{
	{name: "Completions"},
	{name: "Responses", opts: []fantasyopenai.Option{fantasyopenai.WithUseResponsesAPI()}},
}

func newOpenAIModel(t *testing.T, serverURL string, opts []fantasyopenai.Option) fantasy.LanguageModel {
	t.Helper()
	client, err := fantasyopenai.New(append([]fantasyopenai.Option{
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	}, opts...)...)
	require.NoError(t, err)
	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)
	return model
}

func TestOpenAI_StreamingText(t *testing.T) {
	t.Parallel()

	for _, api := range openAIAPIs {
		t.Run(api.name, func(t *testing.T) {
			t.Parallel()

			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				return chattest.OpenAIStreamingResponse(
					append(
						append(
							chattest.OpenAITextChunks("Hello", "Hi"),
							chattest.OpenAITextChunks(" world", " there")...,
						),
						chattest.OpenAITextChunks("!", "!")...,
					)...,
				)
			})
			model := newOpenAIModel(t, serverURL, api.opts)

			stream, err := model.Stream(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role:    fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{fantasy.TextPart{Text: "Say hello"}},
					},
				},
			})
			require.NoError(t, err)

			var deltas []string
			for part := range stream {
				require.NotEqual(t, fantasy.StreamPartTypeError, part.Type, "stream error: %v", part.Error)
				if part.Type == fantasy.StreamPartTypeTextDelta {
					deltas = append(deltas, part.Delta)
				}
			}
			require.Equal(t, []string{"Hello", "Hi", " world", " there", "!", "!"}, deltas)
		})
	}
}

func TestOpenAI_NonStreamingText(t *testing.T) {
	t.Parallel()

	for _, api := range openAIAPIs {
		t.Run(api.name, func(t *testing.T) {
			t.Parallel()

			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				return chattest.OpenAINonStreamingResponse("First response")
			})
			model := newOpenAIModel(t, serverURL, api.opts)

			response, err := model.Generate(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role:    fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{fantasy.TextPart{Text: "Test message"}},
					},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, response)
			require.Equal(t, "First response", response.Content.Text())
		})
	}
}

func TestOpenAI_ToolCalls(t *testing.T) {
	t.Parallel()

	for _, api := range openAIAPIs {
		t.Run(api.name, func(t *testing.T) {
			t.Parallel()

			var requestCount atomic.Int32
			serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
				switch requestCount.Add(1) {
				case 1:
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAIToolCallChunk("get_weather", `{"location":"San Francisco"}`),
					)
				default:
					return chattest.OpenAIStreamingResponse(
						chattest.OpenAITextChunks("The weather in San Francisco is 72F.")...,
					)
				}
			})
			model := newOpenAIModel(t, serverURL, api.opts)

			type weatherInput struct {
				Location string `json:"location"`
			}
			var toolCallCount atomic.Int32
			weatherTool := fantasy.NewAgentTool(
				"get_weather",
				"Get weather for a location.",
				func(ctx context.Context, input weatherInput, _ fantasy.ToolCall) (fantasy.ToolResponse, error) {
					toolCallCount.Add(1)
					require.Equal(t, "San Francisco", input.Location)
					return fantasy.NewTextResponse("72F"), nil
				},
			)

			agent := fantasy.NewAgent(
				model,
				fantasy.WithSystemPrompt("You are a helpful assistant."),
				fantasy.WithTools(weatherTool),
			)

			result, err := agent.Stream(context.Background(), fantasy.AgentStreamCall{
				Prompt: "What's the weather in San Francisco?",
			})
			require.NoError(t, err)
			require.NotNil(t, result)
			require.Equal(t, int32(1), toolCallCount.Load(), "expected exactly one tool execution")
			require.GreaterOrEqual(t, requestCount.Load(), int32(2), "expected follow-up model call after tool execution")
		})
	}
}

func TestOpenAI_Streaming_MismatchReturnsErrorPart(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAINonStreamingResponse("wrong response type")
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	stream, err := model.Stream(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.NoError(t, err)

	var streamErr error
	for part := range stream {
		if part.Type == fantasy.StreamPartTypeError {
			streamErr = part.Error
			break
		}
	}
	require.Error(t, streamErr)
	require.Contains(t, streamErr.Error(), "non-streaming response for streaming request")
}

func TestOpenAI_NonStreaming_MismatchReturnsError_CompletionsAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("wrong response type")...)
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "streaming response for non-streaming request")
}

func TestOpenAI_NonStreaming_MismatchReturnsError_ResponsesAPI(t *testing.T) {
	t.Parallel()

	serverURL := chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
		return chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("wrong response type")...)
	})

	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
		fantasyopenai.WithUseResponsesAPI(),
	)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), "gpt-4")
	require.NoError(t, err)

	_, err = model.Generate(context.Background(), fantasy.Call{
		Prompt: []fantasy.Message{
			{
				Role:    fantasy.MessageRoleUser,
				Content: []fantasy.MessagePart{fantasy.TextPart{Text: "hello"}},
			},
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "streaming response for non-streaming request")
}

func TestOpenAI_ResponsesAPI_UnstoredItemReference(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		store      bool
		refs       []string
		wantStatus int
	}{
		{name: "UnstoredReasoning", refs: []string{"rs_served"}, wantStatus: http.StatusNotFound},
		{name: "UnstoredWebSearch", refs: []string{"ws_served"}, wantStatus: http.StatusNotFound},
		{name: "Stored", store: true, refs: []string{"rs_served", "ws_served"}, wantStatus: http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			serverURL := chattest.NewOpenAI(t, func(*chattest.OpenAIRequest) chattest.OpenAIResponse {
				resp := chattest.OpenAIStreamingResponse(chattest.OpenAITextChunks("done")...)
				resp.Reasoning = &chattest.OpenAIReasoningItem{ID: "rs_served", EncryptedContent: "encrypted"}
				resp.WebSearch = &chattest.OpenAIWebSearchCall{ID: "ws_served", Queries: []string{"go"}}
				return resp
			})
			post := func(store bool, input []any) int {
				body, err := json.Marshal(map[string]any{
					"model": "gpt-4", "stream": true, "store": store, "input": input,
				})
				require.NoError(t, err)
				req, err := http.NewRequestWithContext(testutil.Context(t, testutil.WaitShort), http.MethodPost, serverURL+"/responses", bytes.NewReader(body))
				require.NoError(t, err)
				resp, err := http.DefaultClient.Do(req)
				require.NoError(t, err)
				_, err = io.Copy(io.Discard, resp.Body)
				require.NoError(t, err)
				require.NoError(t, resp.Body.Close())
				return resp.StatusCode
			}

			user := map[string]any{"role": "user", "content": "search"}
			require.Equal(t, http.StatusOK, post(tc.store, []any{user}))
			// openai-go leaves out the optional type on item references.
			input := []any{user}
			for _, id := range tc.refs {
				input = append(input, map[string]any{"id": id})
			}
			require.Equal(t, tc.wantStatus, post(true, input))
		})
	}
}
