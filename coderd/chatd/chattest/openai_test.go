package chattest_test

import (
	"context"
	"sync/atomic"
	"testing"

	"charm.land/fantasy"
	fantasyopenai "charm.land/fantasy/providers/openai"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/chatd/chattest"
)

type openAIAPIMode string

const (
	openAICompletionsAPI openAIAPIMode = "completions"
	openAIResponsesAPI   openAIAPIMode = "responses"
)

func newOpenAILanguageModel(
	t *testing.T,
	mode providerRunMode,
	mockServerURL string,
	apiMode openAIAPIMode,
) fantasy.LanguageModel {
	t.Helper()

	apiKey := "test-key"
	modelName := "gpt-4"
	opts := []fantasyopenai.Option{}

	if mode.Live {
		apiKey = envOrDefault("OPENAI_API_KEY", "")
		modelName = envOrDefault("OPENAI_TEST_MODEL", "gpt-4o-mini")
		if baseURL := envOrDefault("OPENAI_BASE_URL", ""); baseURL != "" {
			opts = append(opts, fantasyopenai.WithBaseURL(baseURL))
		}
	} else {
		opts = append(opts, fantasyopenai.WithBaseURL(mockServerURL))
	}

	opts = append(opts, fantasyopenai.WithAPIKey(apiKey))
	if apiMode == openAIResponsesAPI {
		opts = append(opts, fantasyopenai.WithUseResponsesAPI())
	}

	client, err := fantasyopenai.New(opts...)
	require.NoError(t, err)

	model, err := client.LanguageModel(context.Background(), modelName)
	require.NoError(t, err)
	return model
}

func TestOpenAI_Streaming(t *testing.T) {
	t.Parallel()

	for _, mode := range providerModes(t, "OPENAI_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
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
			}

			model := newOpenAILanguageModel(t, mode, serverURL, openAICompletionsAPI)
			stream, err := model.Stream(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Say hello"},
						},
					},
				},
			})
			require.NoError(t, err)

			var streamErr error
			expectedDeltas := []string{"Hello", "Hi", " world", " there", "!", "!"}
			deltaIndex := 0
			sawTextDelta := false

			for part := range stream {
				if part.Type == fantasy.StreamPartTypeError {
					streamErr = part.Error
					continue
				}
				if part.Type != fantasy.StreamPartTypeTextDelta {
					continue
				}
				sawTextDelta = true
				if mode.Live {
					continue
				}
				require.Less(t, deltaIndex, len(expectedDeltas), "Received more deltas than expected")
				require.Equal(t, expectedDeltas[deltaIndex], part.Delta,
					"Delta at index %d should be %q, got %q", deltaIndex, expectedDeltas[deltaIndex], part.Delta)
				deltaIndex++
			}

			require.NoError(t, streamErr)
			require.True(t, sawTextDelta)
			if !mode.Live {
				require.Equal(t, len(expectedDeltas), deltaIndex, "Expected %d deltas, got %d", len(expectedDeltas), deltaIndex)
			}
		})
	}
}

func TestOpenAI_Streaming_ResponsesAPI(t *testing.T) {
	t.Parallel()

	for _, mode := range providerModes(t, "OPENAI_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
					return chattest.OpenAIStreamingResponse(
						append(
							append(
								chattest.OpenAITextChunks("First", "Second"),
								chattest.OpenAITextChunks(" output", " output")...,
							),
							chattest.OpenAITextChunks("!", "!")...,
						)...,
					)
				})
			}

			model := newOpenAILanguageModel(t, mode, serverURL, openAIResponsesAPI)
			stream, err := model.Stream(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Say hello"},
						},
					},
				},
			})
			require.NoError(t, err)

			var parts []fantasy.StreamPart
			var streamErr error
			for part := range stream {
				parts = append(parts, part)
				if part.Type == fantasy.StreamPartTypeError {
					streamErr = part.Error
				}
			}

			require.NoError(t, streamErr)
			require.Greater(t, len(parts), 0)

			if mode.Live {
				return
			}

			var allDeltas []string
			for _, part := range parts {
				if part.Type == fantasy.StreamPartTypeTextDelta {
					allDeltas = append(allDeltas, part.Delta)
				}
			}

			if len(allDeltas) > 0 {
				allText := ""
				for _, delta := range allDeltas {
					allText += delta
				}
				require.Contains(t, allText, "First")
				require.Contains(t, allText, "Second")
				require.Contains(t, allText, "output")
				require.Contains(t, allText, "!")
			}
		})
	}
}

func TestOpenAI_NonStreaming_CompletionsAPI(t *testing.T) {
	t.Parallel()

	for _, mode := range providerModes(t, "OPENAI_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
					return chattest.OpenAINonStreamingResponse("First response")
				})
			}

			model := newOpenAILanguageModel(t, mode, serverURL, openAICompletionsAPI)
			response, err := model.Generate(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Test message"},
						},
					},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, response)
		})
	}
}

func TestOpenAI_ToolCalls(t *testing.T) {
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

	// Create fantasy client pointing to our test server
	client, err := fantasyopenai.New(
		fantasyopenai.WithAPIKey("test-key"),
		fantasyopenai.WithBaseURL(serverURL),
	)
	require.NoError(t, err)

	ctx := context.Background()
	model, err := client.LanguageModel(ctx, "gpt-4")
	require.NoError(t, err)

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

	result, err := agent.Stream(ctx, fantasy.AgentStreamCall{
		Prompt: "What's the weather in San Francisco?",
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.Equal(t, int32(1), toolCallCount.Load(), "expected exactly one tool execution")
	require.GreaterOrEqual(t, requestCount.Load(), int32(2), "expected follow-up model call after tool execution")
}

func TestOpenAI_NonStreaming_ResponsesAPI(t *testing.T) {
	t.Parallel()

	for _, mode := range providerModes(t, "OPENAI_API_KEY") {
		mode := mode
		t.Run(mode.Name, func(t *testing.T) {
			t.Parallel()

			serverURL := ""
			if !mode.Live {
				serverURL = chattest.NewOpenAI(t, func(req *chattest.OpenAIRequest) chattest.OpenAIResponse {
					return chattest.OpenAINonStreamingResponse("First output")
				})
			}

			model := newOpenAILanguageModel(t, mode, serverURL, openAIResponsesAPI)
			response, err := model.Generate(context.Background(), fantasy.Call{
				Prompt: []fantasy.Message{
					{
						Role: fantasy.MessageRoleUser,
						Content: []fantasy.MessagePart{
							fantasy.TextPart{Text: "Test message"},
						},
					},
				},
			})
			require.NoError(t, err)
			require.NotNil(t, response)
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
